#!/usr/bin/env python3
"""Build self-contained compatibility SVGs from Serenity's selected raster mark."""

from __future__ import annotations

import base64
from pathlib import Path
import struct

ROOT = Path(__file__).resolve().parents[2]
ASSETS = ROOT / "site" / "assets"
SOURCE = ASSETS / "bookbinder-rose.png"


def svg(title: str, view_box: str, content: str) -> str:
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" '
        f'viewBox="{view_box}" role="img" aria-labelledby="title">'
        f'<title id="title">{title}</title>{content}</svg>\n'
    )


def main() -> None:
    raw = SOURCE.read_bytes()
    if raw[:8] != b"\x89PNG\r\n\x1a\n" or raw[12:16] != b"IHDR":
        raise SystemExit(f"{SOURCE} is not a valid PNG")
    width, height = struct.unpack(">II", raw[16:24])
    png = base64.b64encode(raw).decode("ascii")
    data = f"data:image/png;base64,{png}"
    # Crop only transparent canvas space in the SVG viewBox. The PNG pixels
    # and silhouette remain unchanged; this keeps the mark legible at 16-36px.
    crop_x, crop_y = round(width * 0.106), round(height * 0.035)
    crop_w, crop_h = round(width * 0.751), round(height * 0.93)
    view_box = f"{crop_x} {crop_y} {crop_w} {crop_h}"
    image = (
        f'<image x="0" y="0" width="{width}" height="{height}" '
        f'preserveAspectRatio="xMidYMid meet" href="{data}"/>'
    )
    ASSETS.joinpath("brand.svg").write_text(
        svg("Serenity Book Binder mark in rose", view_box, image),
        encoding="utf-8",
    )
    for name, title, rgb in (
        ("brand-light.svg", "Serenity Book Binder mark in light ink", (0.95, 0.91, 0.92)),
        ("brand-mono.svg", "Serenity Book Binder mark in monochrome charcoal", (0.13, 0.12, 0.13)),
    ):
        matrix = (
            f"0 0 0 0 {rgb[0]} 0 0 0 0 {rgb[1]} "
            f"0 0 0 0 {rgb[2]} 0 0 0 1 0"
        )
        content = (
            f'<defs><filter id="brand-tint" color-interpolation-filters="sRGB">'
            f'<feColorMatrix type="matrix" values="{matrix}"/>'
            f"</filter></defs><g filter=\"url(#brand-tint)\">{image}</g>"
        )
        ASSETS.joinpath(name).write_text(svg(title, view_box, content), encoding="utf-8")
    wordmark = (
        f'<image x="0" y="0" width="120" height="120" '
        f'preserveAspectRatio="xMidYMid meet" href="{data}"/>'
        '<text x="130" y="83" fill="#f1edeb" '
        'font-family="Inter, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif" '
        'font-size="64" font-weight="600" letter-spacing="-2">Serenity</text>'
    )
    ASSETS.joinpath("wordmark.svg").write_text(
        svg("Serenity wordmark with the rose Book Binder mark", "0 0 470 120", wordmark),
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
