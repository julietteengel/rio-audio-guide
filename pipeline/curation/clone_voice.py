"""One-off admin script: clone a voice on ElevenLabs from a local audio sample.

Usage:
    ELEVENLABS_API_KEY=... python clone_voice.py --name "Julie" --sample /path/to/sample.mp3

Prints the resulting voice_id, to be stored in config/secrets and passed manually
later at script-review time (POST /scripts/{id}/review) — this script has no other
effect and is not called by the import or the TTS worker.

1-2 minutes of clean audio (quiet room, consistent mic) in a single language is
enough: ElevenLabs' multilingual model speaks the cloned voice in 32+ languages
from translated text, it does not need one sample per language.
"""
import argparse
import os
import sys

import requests

API_URL = "https://api.elevenlabs.io/v1/voices/add"


def clone_voice(api_key, name, sample_path):
    with open(sample_path, "rb") as f:
        response = requests.post(
            API_URL,
            headers={"xi-api-key": api_key},
            data={"name": name},
            files={"files": (os.path.basename(sample_path), f)},
            timeout=60,
        )
    response.raise_for_status()
    return response.json()["voice_id"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--name", required=True, help="Name to give the cloned voice on ElevenLabs")
    parser.add_argument("--sample", required=True, help="Path to a clean audio sample (1-2 min, wav/mp3)")
    args = parser.parse_args()

    api_key = os.environ.get("ELEVENLABS_API_KEY")
    if not api_key:
        print("ELEVENLABS_API_KEY is not set", file=sys.stderr)
        sys.exit(1)

    voice_id = clone_voice(api_key, args.name, args.sample)
    print(f"voice_id: {voice_id}")


if __name__ == "__main__":
    main()
