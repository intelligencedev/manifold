#!/usr/bin/env python3

import argparse
from pathlib import Path

from PIL import Image, ImageOps


def _flatten_alpha(img: Image.Image, background_rgb=(255, 255, 255)) -> Image.Image:
    if img.mode in {"RGBA", "LA"} or (img.mode == "P" and "transparency" in img.info):
        rgba = img.convert("RGBA")
        bg = Image.new("RGBA", rgba.size, background_rgb + (255,))
        return Image.alpha_composite(bg, rgba).convert("RGB")
    return img.convert("RGB")


def convert_to_webp_1440p(
    input_path: Path,
    output_path: Path,
    *,
    max_width: int,
    max_height: int,
    quality: int,
    method: int,
) -> None:
    with Image.open(input_path) as img:
        img = ImageOps.exif_transpose(img)
        img = _flatten_alpha(img)

        original_size = img.size
        img.thumbnail((max_width, max_height), resample=Image.Resampling.LANCZOS)
        resized_size = img.size

        output_path.parent.mkdir(parents=True, exist_ok=True)
        img.save(
            output_path,
            format="WEBP",
            quality=quality,
            method=method,
            optimize=True,
        )

    print(f"Input : {input_path}")
    print(f"Output: {output_path}")
    print(f"Size  : {original_size[0]}x{original_size[1]} -> {resized_size[0]}x{resized_size[1]}")


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Convert an image to WebP and resize it to fit within a 1440p (2560x1440) bounding box. "
            "Aspect ratio is preserved and images are never upscaled."
        )
    )
    parser.add_argument(
        "input",
        nargs="?",
        default="docs/img/chat.png",
        help="Input image path (default: docs/img/chat.png)",
    )
    parser.add_argument(
        "-o",
        "--output",
        default=None,
        help=(
            "Output WebP path. Defaults to <input-stem>-1440p.webp next to the input. "
            "Example: docs/img/chat-1440p.webp"
        ),
    )
    parser.add_argument("--max-width", type=int, default=2560)
    parser.add_argument("--max-height", type=int, default=1440)
    parser.add_argument("--quality", type=int, default=82)
    parser.add_argument("--method", type=int, default=6, help="WebP encoder effort 0-6")

    args = parser.parse_args()

    input_path = Path(args.input)
    if not input_path.exists():
        raise SystemExit(f"Input file not found: {input_path}")

    output_path = Path(args.output) if args.output else input_path.with_name(f"{input_path.stem}-1440p.webp")

    convert_to_webp_1440p(
        input_path,
        output_path,
        max_width=args.max_width,
        max_height=args.max_height,
        quality=args.quality,
        method=args.method,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
