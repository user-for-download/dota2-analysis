"""CLI entry point for the trainer."""
import click
from trainer.config import Settings


@click.group()
@click.pass_context
def cli(ctx):
    """Dota 2 draft analysis model trainer."""
    ctx.ensure_object(dict)
    ctx.obj["settings"] = Settings()


@cli.command()
@click.pass_context
def extract(ctx):
    """Extract training data from Postgres to Parquet."""
    from trainer.extract import run
    run(ctx.obj["settings"])


@cli.command()
@click.pass_context
def train_imitation(ctx):
    """Train imitation model (lambdarank)."""
    from trainer.train_imitation import run
    run(ctx.obj["settings"])


@cli.command()
@click.pass_context
def train_value(ctx):
    """Train value model (binary classification)."""
    from trainer.train_value import run
    run(ctx.obj["settings"])


@cli.command()
@click.pass_context
def evaluate(ctx):
    """Evaluate trained models."""
    from trainer.evaluate import run
    run(ctx.obj["settings"])


@cli.command()
@click.pass_context
def publish(ctx):
    """Publish model to deploy/models/ directory."""
    from trainer.publish import run
    run(ctx.obj["settings"])


@cli.command()
@click.pass_context
def pipeline(ctx):
    """Run full pipeline: extract -> quality -> train -> evaluate -> gate -> publish."""
    from trainer.extract import run as extract_run
    from trainer.quality import run as quality_run
    from trainer.train_imitation import run as train_imit_run
    from trainer.train_value import run as train_value_run
    from trainer.evaluate import run as eval_run
    from trainer.gate import run as gate_run
    from trainer.publish import run as publish_run

    settings = ctx.obj["settings"]
    extract_run(settings)
    quality_run(settings)
    train_imit_run(settings)
    train_value_run(settings)
    eval_run(settings)
    gate_run(settings)
    publish_run(settings)


@cli.command()
@click.pass_context
def diagnose(ctx):
    """Run MV population diagnostic SQL and print results."""
    import pandas as pd
    from sqlalchemy import text as sqltext
    from trainer.db import get_engine

    settings = ctx.obj["settings"]
    engine = get_engine(settings)

    queries = {
        "Row counts": sqltext("""
            SELECT 'team_matches' AS tbl, COUNT(*) FROM public.team_matches
            UNION ALL SELECT 'matches (league)', COUNT(*) FROM public.matches WHERE leagueid > 0
            UNION ALL SELECT 'matches (both teams)', COUNT(*) FROM public.matches
                WHERE leagueid > 0 AND radiant_team_id IS NOT NULL AND dire_team_id IS NOT NULL
            UNION ALL SELECT 'picks_bans', COUNT(*) FROM public.picks_bans
            UNION ALL SELECT 'player_matches', COUNT(*) FROM public.player_matches
            UNION ALL SELECT 'heroes', COUNT(*) FROM public.heroes
            UNION ALL SELECT 'matches (last 6mo)', COUNT(*) FROM public.matches
                WHERE leagueid > 0 AND start_time >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT
            ORDER BY tbl;
        """),
        "Match time range": sqltext("""
            SELECT to_timestamp(MIN(start_time)) AS earliest,
                   to_timestamp(MAX(start_time)) AS latest,
                   COUNT(*) FILTER (WHERE start_time >= EXTRACT(EPOCH FROM (NOW() - INTERVAL '6 months'))::BIGINT) AS within_6mo
            FROM public.matches WHERE leagueid > 0;
        """),
        "Joinable picks_bans ↔ team_matches": sqltext("""
            SELECT COUNT(*) AS joinable
            FROM public.picks_bans pb
            JOIN public.matches m ON m.match_id = pb.match_id
            JOIN public.team_matches tm
                ON tm.match_id = pb.match_id
                AND tm.team_id = CASE WHEN pb.team = 0 THEN m.radiant_team_id ELSE m.dire_team_id END
            WHERE pb.is_pick = true AND m.leagueid > 0;
        """),
    }

    for label, sql in queries.items():
        print(f"\n{'='*60}")
        print(f"  {label}")
        print(f"{'='*60}")
        try:
            df = pd.read_sql(sql, engine)
            print(df.to_string(index=False))
        except Exception as e:
            print(f"  ERROR: {e}")

    print()


if __name__ == "__main__":
    cli()
