"""Publish model to deploy/models/ directory."""
import json
import shutil
from pathlib import Path
from trainer.config import Settings


def run(settings: Settings):
    """Publish the trained model atomically via symlink swap.

    1. Copy model files to a versioned directory under model_dir.
    2. Create a temporary symlink pointing to the new version.
    3. Atomically rename the temp symlink to 'current' (POSIX rename
       is atomic on the same filesystem).

    The Go inference service reads from model_dir/imitation/current/,
    so this swap is the seam between Python training and Go inference.
    """
    meta = json.loads(
        (settings.artifact_dir / "imitation" / "meta.json").read_text()
    )
    version = meta["version"]

    # Create versioned directory
    versioned_dir = settings.model_dir / "imitation" / version
    versioned_dir.mkdir(parents=True, exist_ok=True)

    # Copy model files
    for f in ["model.bin", "spec.json", "meta.json"]:
        src = settings.artifact_dir / "imitation" / f
        dst = versioned_dir / f
        shutil.copy2(src, dst)

    # Atomic symlink swap
    current_link = settings.model_dir / "imitation" / "current"
    tmp_link = settings.model_dir / "imitation" / "current.tmp"

    if tmp_link.exists() or tmp_link.is_symlink():
        tmp_link.unlink()
    tmp_link.symlink_to(versioned_dir)
    tmp_link.rename(current_link)

    print(f"Published model {version} -> {current_link}")
