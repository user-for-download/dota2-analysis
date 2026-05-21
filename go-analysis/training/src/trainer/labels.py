"""Label generation for training."""
import pandas as pd


def imitation_labels(decisions: pd.DataFrame) -> pd.DataFrame:
    """Create imitation labels: 1 for chosen hero, 0 for others.

    In the full implementation, this generates negative samples by
    pairing each decision with all undrafted heroes (label=0) alongside
    the actually chosen hero (label=1).
    """
    df = decisions.copy()
    df["label"] = 1.0
    return df


def value_labels(decisions: pd.DataFrame) -> pd.DataFrame:
    """Create value labels: 1 if acting team won, 0 otherwise."""
    df = decisions.copy()
    df["value_label"] = df["acting_won"].astype(int)
    return df
