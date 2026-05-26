"""Publish model to deploy/models/ directory."""
import json
import shutil
from pathlib import Path
from trainer.config import Settings


def _publish_one(artifact_name: str, settings: Settings):
    """Publish a single model (imitation or value) via atomic symlink swap.

    1. Copy model files to a versioned directory under model_dir/{name}/.
    2. Create a temporary symlink pointing to the new version.
    3. Atomically rename the temp symlink to 'current' (POSIX rename
       is atomic on the same filesystem).

    The Go inference service reads from model_dir/{name}/current/,
    so this swap is the seam between Python training and Go inference.
    """
    meta_path = settings.artifact_dir / artifact_name / "meta.json"
    if not meta_path.exists():
        print(f"Skipping {artifact_name}: no meta.json found")
        return

    meta = json.loads(meta_path.read_text())
    version = meta["version"]

    # Create versioned directory
    versioned_dir = settings.model_dir / artifact_name / version
    versioned_dir.mkdir(parents=True, exist_ok=True)

    # Copy model files
    for f in ["model.bin", "spec.json", "meta.json"]:
        src = settings.artifact_dir / artifact_name / f
        dst = versioned_dir / f
        shutil.copy2(src, dst)

    # Atomic symlink swap
    current_link = settings.model_dir / artifact_name / "current"
    tmp_link = settings.model_dir / artifact_name / "current.tmp"

    # Remove stale tmp link if present
    if tmp_link.is_symlink() or tmp_link.exists():
        tmp_link.unlink()

    # If current_link is a real directory (not a symlink), remove it first
    if current_link.exists() and not current_link.is_symlink():
        shutil.rmtree(current_link)

    tmp_link.symlink_to(versioned_dir)
    tmp_link.rename(current_link)

    print(f"Published {artifact_name} model {version} -> {current_link}")


def run(settings: Settings):
    """Publish both imitation and value models."""
    _publish_one("imitation", settings)
    _publish_one("value", settings)
