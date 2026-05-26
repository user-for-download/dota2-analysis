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


if __name__ == "__main__":
    cli()
