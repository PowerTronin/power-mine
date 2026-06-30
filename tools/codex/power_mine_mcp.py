#!/usr/bin/env python3
"""Power Mine diagnostics CLI and MCP server.

The script intentionally uses only the Python standard library so Codex can run
it on a developer machine without installing a separate package first.
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
import re
import shlex
import shutil
import signal
import subprocess
import sys
import time
import traceback
import urllib.error
import urllib.parse
import urllib.request
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any

try:
    import tomllib
except ModuleNotFoundError:  # pragma: no cover - Python < 3.11 fallback
    tomllib = None


MAX_LOG_BYTES = 512 * 1024
MOD_FILE_RE = re.compile(r"^[a-z][a-z0-9_-]{1,63}$")
DEFAULT_AGENT_PORT = 39276
AGENT_MOD_FILE_NAME = "power-mine-agent.jar"
FABRIC_AGENT_TARGET = "fabric-1.20.1"
FORGE_1710_AGENT_TARGET = "forge-1.7.10"
FORGE_1122_AGENT_TARGET = "forge-1.12.2"
AGENT_TARGETS = {
    FABRIC_AGENT_TARGET: {
        "loader": "fabric",
        "minecraftVersion": "1.20.1",
        "libs": ("agent", "build", "libs"),
        "fileName": AGENT_MOD_FILE_NAME,
        "description": "Fabric 1.20.1 runtime agent",
    },
    FORGE_1710_AGENT_TARGET: {
        "loader": "forge",
        "minecraftVersion": "1.7.10",
        "libs": ("forge-agent", "build", "libs"),
        "fileName": "power-mine-forge-1.7.10-agent.jar",
        "description": "Forge 1.7.10 runtime agent",
    },
    FORGE_1122_AGENT_TARGET: {
        "loader": "forge",
        "minecraftVersion": "1.12.2",
        "libs": ("forge-1122-agent", "build", "libs"),
        "fileName": "power-mine-forge-1.12.2-agent.jar",
        "description": "Forge 1.12.2 runtime agent",
    },
}
PROFILE_PROVIDED_MOD_IDS = {"minecraft", "java", "fabricloader", "quilt_loader", "forge", "fml"}
LATEST_RUN_LOG_SLOP_SECONDS = 8


@dataclass
class DiagnosticContext:
    data_dir: Path


def default_data_dir() -> Path:
    if os.environ.get("POWER_MINE_DATA_DIR"):
        return Path(os.environ["POWER_MINE_DATA_DIR"]).expanduser()

    home = Path.home()
    if sys.platform == "darwin":
        return home / "Library" / "Application Support" / "Power Mine"
    if sys.platform.startswith("linux"):
        if os.environ.get("XDG_DATA_HOME"):
            return Path(os.environ["XDG_DATA_HOME"]).expanduser() / "power-mine"
        return home / ".local" / "share" / "power-mine"
    return home / ".power-mine"


def context_from_args(data_dir: str | None = None) -> DiagnosticContext:
    return DiagnosticContext(data_dir=Path(data_dir).expanduser() if data_dir else default_data_dir())


def read_json(path: Path, default: Any) -> Any:
    try:
        with path.open("r", encoding="utf-8") as handle:
            return json.load(handle)
    except FileNotFoundError:
        return default


def load_profiles(ctx: DiagnosticContext) -> dict[str, Any]:
    raw = read_json(ctx.data_dir / "profiles.json", {"selectedProfileId": "", "profiles": []})
    profiles = raw.get("profiles") if isinstance(raw, dict) else []
    if not isinstance(profiles, list):
        profiles = []
    return {
        "dataDir": str(ctx.data_dir),
        "selectedProfileId": raw.get("selectedProfileId", "") if isinstance(raw, dict) else "",
        "profiles": profiles,
    }


def find_profile(ctx: DiagnosticContext, profile_id: str | None) -> dict[str, Any]:
    data = load_profiles(ctx)
    wanted = (profile_id or data.get("selectedProfileId") or "").strip()
    profiles = data.get("profiles", [])
    if not wanted and len(profiles) == 1:
        return profiles[0]
    for profile in profiles:
        if profile.get("id") == wanted:
            return profile
    if wanted:
        raise ValueError(f"profile not found: {wanted}")
    raise ValueError("no profile selected; pass profile_id")


def profile_brief(profile: dict[str, Any]) -> dict[str, Any]:
    loader = profile.get("loader") or {}
    return {
        "id": profile.get("id", ""),
        "name": profile.get("name", ""),
        "minecraftVersion": profile.get("minecraftVersion", ""),
        "loader": loader.get("type", "vanilla"),
        "loaderVersion": loader.get("version", ""),
        "gameDir": profile.get("gameDir", ""),
        "installStatus": (profile.get("install") or {}).get("status", ""),
    }


def sha1_file(path: Path) -> str:
    digest = hashlib.sha1()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def as_list(value: Any) -> list[Any]:
    if value is None:
        return []
    if isinstance(value, list):
        return value
    return [value]


def read_zip_json(jar: zipfile.ZipFile, name: str) -> tuple[dict[str, Any] | None, str | None]:
    try:
        with jar.open(name) as handle:
            raw = handle.read().decode("utf-8")
        data = json.loads(raw)
        if isinstance(data, dict):
            return data, None
        return None, f"{name} is not a JSON object"
    except KeyError:
        return None, None
    except Exception as exc:
        return None, f"cannot parse {name}: {exc}"


def read_zip_text(jar: zipfile.ZipFile, name: str) -> tuple[str | None, str | None]:
    try:
        with jar.open(name) as handle:
            return handle.read().decode("utf-8", errors="replace"), None
    except KeyError:
        return None, None
    except Exception as exc:
        return None, f"cannot read {name}: {exc}"


def parse_toml(text: str) -> dict[str, Any]:
    if tomllib is None:
        return parse_toml_fallback(text)
    try:
        data = tomllib.loads(text)
        return data if isinstance(data, dict) else {}
    except Exception:
        return parse_toml_fallback(text)


def parse_toml_fallback(text: str) -> dict[str, Any]:
    result: dict[str, Any] = {}
    mods = []
    for block in re.split(r"(?m)^\s*\[\[mods\]\]\s*$", text)[1:]:
        item: dict[str, str] = {}
        for key, value in re.findall(r'(?m)^\s*([A-Za-z0-9_.-]+)\s*=\s*"([^"]*)"', block):
            item[key] = value
        if item:
            mods.append(item)
    if mods:
        result["mods"] = mods
    for key in ("modLoader", "loaderVersion", "license", "issueTrackerURL"):
        match = re.search(rf'(?m)^\s*{re.escape(key)}\s*=\s*"([^"]*)"', text)
        if match:
            result[key] = match.group(1)
    return result


def fabric_dependencies(metadata: dict[str, Any]) -> dict[str, Any]:
    deps: dict[str, Any] = {}
    for section in ("depends", "breaks", "recommends", "suggests"):
        value = metadata.get(section)
        if isinstance(value, dict):
            deps[section] = value
    return deps


def inspect_fabric_metadata(path: str, metadata: dict[str, Any], names: set[str]) -> dict[str, Any]:
    mod_id = str(metadata.get("id", ""))
    result = {
        "loader": "fabric",
        "metadataFile": path,
        "id": mod_id,
        "name": metadata.get("name") or mod_id,
        "version": metadata.get("version", ""),
        "environment": metadata.get("environment", "*"),
        "depends": fabric_dependencies(metadata),
        "entrypoints": metadata.get("entrypoints", {}),
        "mixins": metadata.get("mixins", []),
        "accessWidener": metadata.get("accessWidener", ""),
        "issues": [],
        "warnings": [],
    }
    if mod_id and not MOD_FILE_RE.match(mod_id):
        result["warnings"].append(f"Fabric mod id has unusual format: {mod_id}")

    for mixin in as_list(metadata.get("mixins")):
        mixin_path = mixin.get("config") if isinstance(mixin, dict) else mixin
        if isinstance(mixin_path, str) and mixin_path and mixin_path not in names:
            result["issues"].append(f"Referenced mixin config is missing: {mixin_path}")

    widener = metadata.get("accessWidener")
    if isinstance(widener, str) and widener and widener not in names:
        result["issues"].append(f"Referenced access widener is missing: {widener}")
    return result


def fabric_nested_jar_paths(metadata: dict[str, Any], names: set[str]) -> list[str]:
    paths: set[str] = set()
    jars = metadata.get("jars")
    if isinstance(jars, list):
        for item in jars:
            nested_path = item.get("file") if isinstance(item, dict) else item
            if isinstance(nested_path, str) and nested_path:
                paths.add(nested_path)
    paths.update(name for name in names if name.startswith("META-INF/jars/") and name.lower().endswith(".jar"))
    return sorted(paths)


def inspect_nested_fabric_jars(
    jar: zipfile.ZipFile,
    owner_path: str,
    owner_metadata: dict[str, Any],
    owner_names: set[str],
) -> tuple[list[dict[str, Any]], list[str]]:
    metadata_items: list[dict[str, Any]] = []
    issues: list[str] = []
    for nested_path in fabric_nested_jar_paths(owner_metadata, owner_names):
        if nested_path not in owner_names:
            issues.append(f"Referenced nested jar is missing: {nested_path}")
            continue
        try:
            with jar.open(nested_path) as handle:
                raw = handle.read()
            with zipfile.ZipFile(io.BytesIO(raw)) as nested_jar:
                nested_names = set(nested_jar.namelist())
                nested_metadata, error = read_zip_json(nested_jar, "fabric.mod.json")
                if error:
                    issues.append(f"{nested_path}: {error}")
                if nested_metadata:
                    metadata_items.append(
                        inspect_fabric_metadata(f"{owner_path}!/{nested_path}!/fabric.mod.json", nested_metadata, nested_names)
                    )
        except zipfile.BadZipFile:
            issues.append(f"Nested jar is not a readable jar/zip archive: {nested_path}")
        except Exception as exc:
            issues.append(f"Cannot inspect nested jar {nested_path}: {exc}")
    return metadata_items, issues


def inspect_quilt_metadata(path: str, metadata: dict[str, Any], names: set[str]) -> dict[str, Any]:
    loader = metadata.get("quilt_loader") if isinstance(metadata.get("quilt_loader"), dict) else {}
    meta = loader.get("metadata") if isinstance(loader.get("metadata"), dict) else {}
    mod_id = str(loader.get("id", ""))
    result = {
        "loader": "quilt",
        "metadataFile": path,
        "id": mod_id,
        "name": meta.get("name") or mod_id,
        "version": loader.get("version", ""),
        "depends": {"depends": loader.get("depends", [])},
        "entrypoints": metadata.get("entrypoints", {}),
        "mixins": metadata.get("mixin", []),
        "issues": [],
        "warnings": [],
    }
    if mod_id and not MOD_FILE_RE.match(mod_id):
        result["warnings"].append(f"Quilt mod id has unusual format: {mod_id}")
    for mixin in as_list(metadata.get("mixin")):
        if isinstance(mixin, str) and mixin and mixin not in names:
            result["issues"].append(f"Referenced mixin config is missing: {mixin}")
    return result


def inspect_forge_metadata(path: str, text: str, loader: str) -> dict[str, Any]:
    data = parse_toml(text)
    mods = data.get("mods", [])
    first_mod = mods[0] if isinstance(mods, list) and mods else {}
    dependencies = []
    raw_dependencies = data.get("dependencies")
    if isinstance(raw_dependencies, dict):
        for mod_id, entries in raw_dependencies.items():
            for entry in as_list(entries):
                if isinstance(entry, dict):
                    item = dict(entry)
                    item.setdefault("owner", mod_id)
                    dependencies.append(item)
    return {
        "loader": loader,
        "metadataFile": path,
        "id": first_mod.get("modId", "") if isinstance(first_mod, dict) else "",
        "name": first_mod.get("displayName", "") if isinstance(first_mod, dict) else "",
        "version": first_mod.get("version", "") if isinstance(first_mod, dict) else "",
        "modLoader": data.get("modLoader", ""),
        "loaderVersion": data.get("loaderVersion", ""),
        "dependencies": dependencies,
        "issues": [],
        "warnings": [],
    }


def inspect_legacy_forge_metadata(path: str, metadata: Any) -> dict[str, Any]:
    first_mod: dict[str, Any] = {}
    if isinstance(metadata, list) and metadata and isinstance(metadata[0], dict):
        first_mod = metadata[0]
    elif isinstance(metadata, dict):
        first_mod = metadata
    return {
        "loader": "forge",
        "metadataFile": path,
        "id": first_mod.get("modid", "") or first_mod.get("modId", ""),
        "name": first_mod.get("name", ""),
        "version": first_mod.get("version", ""),
        "mcversion": first_mod.get("mcversion", ""),
        "legacy": True,
        "dependencies": [],
        "issues": [],
        "warnings": [],
    }


def resource_id(value: Any, default_namespace: str = "minecraft") -> tuple[str, str] | None:
    if not isinstance(value, str):
        return None
    value = value.strip()
    if not value or value.startswith("#"):
        return None
    if ":" in value:
        namespace, path = value.split(":", 1)
    else:
        namespace, path = default_namespace, value
    if not namespace or not path:
        return None
    return namespace, path


def model_resource_path(model_id: tuple[str, str]) -> str:
    namespace, path = model_id
    return f"assets/{namespace}/models/{path}.json"


def texture_resource_path(texture_id: tuple[str, str]) -> str:
    namespace, path = texture_id
    return f"assets/{namespace}/textures/{path}.png"


def item_model_resource_path(item_id: tuple[str, str]) -> str:
    namespace, path = item_id
    return f"assets/{namespace}/models/item/{path}.json"


def collect_asset_namespaces(names: set[str]) -> set[str]:
    namespaces = set()
    for name in names:
        parts = name.split("/")
        if len(parts) >= 3 and parts[0] == "assets":
            namespaces.add(parts[1])
    return namespaces


def collect_data_namespaces(names: set[str]) -> set[str]:
    namespaces = set()
    for name in names:
        parts = name.split("/")
        if len(parts) >= 3 and parts[0] == "data":
            namespaces.add(parts[1])
    return namespaces


def should_require_pack_resource(namespace: str, asset_namespaces: set[str]) -> bool:
    """Return whether a referenced asset should be bundled in the inspected jar.

    Vanilla assets are supplied by Minecraft itself, so a mod jar can validly
    reference minecraft:block/stone or minecraft:iron_ingot without bundling
    those resources.
    """
    return namespace != "minecraft" and namespace in asset_namespaces


def inspect_model_file(
    path: str,
    data: dict[str, Any],
    names: set[str],
    asset_namespaces: set[str],
) -> tuple[list[str], list[str]]:
    issues: list[str] = []
    warnings: list[str] = []
    parts = path.split("/")
    namespace = parts[1] if len(parts) > 2 and parts[0] == "assets" else "minecraft"

    parent = data.get("parent")
    parent_id = resource_id(parent, namespace)
    if parent_id and parent_id[0] != "minecraft" and model_resource_path(parent_id) not in names:
        issues.append(f"{path}: parent model is missing: {parent}")

    textures = data.get("textures")
    if isinstance(textures, dict):
        for key, value in textures.items():
            texture_id = resource_id(value, namespace)
            if texture_id is None:
                continue
            texture_path = texture_resource_path(texture_id)
            if should_require_pack_resource(texture_id[0], asset_namespaces) and texture_path not in names:
                issues.append(f"{path}: texture {key!r} is missing: {value}")
    elif textures is not None:
        warnings.append(f"{path}: textures must be an object")

    return issues, warnings


def blockstate_model_values(data: Any) -> list[str]:
    values: list[str] = []
    if isinstance(data, dict):
        model = data.get("model")
        if isinstance(model, str):
            values.append(model)
        for value in data.values():
            values.extend(blockstate_model_values(value))
    elif isinstance(data, list):
        for item in data:
            values.extend(blockstate_model_values(item))
    return values


def inspect_blockstate_file(path: str, data: dict[str, Any], names: set[str], asset_namespaces: set[str]) -> tuple[list[str], list[str]]:
    issues: list[str] = []
    warnings: list[str] = []
    parts = path.split("/")
    namespace = parts[1] if len(parts) > 2 and parts[0] == "assets" else "minecraft"
    for model in blockstate_model_values(data):
        model_id = resource_id(model, namespace)
        if model_id is None:
            continue
        if should_require_pack_resource(model_id[0], asset_namespaces) and model_resource_path(model_id) not in names:
            issues.append(f"{path}: blockstate model is missing: {model}")
    if not blockstate_model_values(data):
        warnings.append(f"{path}: no model references found")
    return issues, warnings


def recipe_result_item(data: dict[str, Any]) -> tuple[str, int] | None:
    result = data.get("result")
    if isinstance(result, str):
        return result, 1
    if isinstance(result, dict):
        item = result.get("item")
        if isinstance(item, str):
            count = result.get("count", 1)
            return item, int(count) if isinstance(count, int) else 1
    return None


def ingredient_items(value: Any) -> list[str]:
    items: list[str] = []
    if isinstance(value, dict):
        item = value.get("item")
        if isinstance(item, str):
            items.append(item)
        for child in ("items", "ingredient", "ingredients"):
            if child in value:
                items.extend(ingredient_items(value[child]))
    elif isinstance(value, list):
        for item in value:
            items.extend(ingredient_items(item))
    return items


def validate_recipe_identifiers(path: str, ids: list[str], issues: list[str], warnings: list[str]) -> None:
    for item in ids:
        parsed = resource_id(item, "minecraft")
        if parsed is None:
            issues.append(f"{path}: invalid item id in recipe: {item!r}")
        elif not re.match(r"^[a-z0-9_.-]+$", parsed[0]) or not re.match(r"^[a-z0-9_./-]+$", parsed[1]):
            warnings.append(f"{path}: unusual item id in recipe: {item!r}")


def inspect_recipe_file(path: str, data: dict[str, Any], names: set[str], asset_namespaces: set[str]) -> tuple[list[str], list[str], dict[str, Any]]:
    issues: list[str] = []
    warnings: list[str] = []
    recipe_type = str(data.get("type", ""))
    summary: dict[str, Any] = {"path": path, "type": recipe_type}

    result_item = recipe_result_item(data)
    if result_item is None:
        issues.append(f"{path}: recipe result item is missing")
    else:
        item_id, count = result_item
        summary["result"] = {"item": item_id, "count": count}
        parsed = resource_id(item_id, "minecraft")
        validate_recipe_identifiers(path, [item_id], issues, warnings)
        if parsed and should_require_pack_resource(parsed[0], asset_namespaces) and item_model_resource_path(parsed) not in names:
            issues.append(f"{path}: result item has no item model: {item_id}")

    if recipe_type == "minecraft:crafting_shaped":
        pattern = data.get("pattern")
        key = data.get("key")
        if not isinstance(pattern, list) or not pattern or not all(isinstance(row, str) for row in pattern):
            issues.append(f"{path}: shaped recipe pattern must be a non-empty string array")
        else:
            width = len(pattern[0])
            if width == 0 or any(len(row) != width for row in pattern):
                issues.append(f"{path}: shaped recipe pattern rows must have equal non-zero width")
            used_chars = {char for row in pattern for char in row if char != " "}
            if not isinstance(key, dict):
                issues.append(f"{path}: shaped recipe key must be an object")
            else:
                key_chars = set(key.keys())
                for char in sorted(used_chars - key_chars):
                    issues.append(f"{path}: shaped recipe pattern uses undefined key {char!r}")
                for char in sorted(key_chars - used_chars):
                    warnings.append(f"{path}: shaped recipe key {char!r} is unused")
                validate_recipe_identifiers(path, ingredient_items(key), issues, warnings)
            summary["width"] = width
            summary["height"] = len(pattern)
    elif recipe_type == "minecraft:crafting_shapeless":
        ingredients = data.get("ingredients")
        if not isinstance(ingredients, list) or not ingredients:
            issues.append(f"{path}: shapeless recipe ingredients must be a non-empty array")
        else:
            validate_recipe_identifiers(path, ingredient_items(ingredients), issues, warnings)
            summary["ingredients"] = len(ingredients)
    elif recipe_type.startswith("minecraft:crafting_"):
        warnings.append(f"{path}: crafting recipe type is not explicitly supported by static validator: {recipe_type}")
    elif recipe_type:
        warnings.append(f"{path}: non-crafting recipe skipped by static validator: {recipe_type}")
    else:
        issues.append(f"{path}: recipe type is missing")

    return issues, warnings, summary


def inspect_mod_content(jar: zipfile.ZipFile, names: set[str]) -> dict[str, Any]:
    asset_namespaces = collect_asset_namespaces(names)
    data_namespaces = collect_data_namespaces(names)
    asset_issues: list[str] = []
    asset_warnings: list[str] = []
    recipe_issues: list[str] = []
    recipe_warnings: list[str] = []
    recipe_summaries: list[dict[str, Any]] = []

    model_paths = sorted(
        name
        for name in names
        if name.startswith("assets/") and "/models/" in name and name.endswith(".json")
    )
    blockstate_paths = sorted(
        name
        for name in names
        if name.startswith("assets/") and "/blockstates/" in name and name.endswith(".json")
    )
    texture_paths = sorted(
        name
        for name in names
        if name.startswith("assets/") and "/textures/" in name and name.endswith(".png")
    )
    recipe_paths = sorted(
        name
        for name in names
        if name.startswith("data/") and "/recipes/" in name and name.endswith(".json")
    )

    for path in model_paths:
        data, error = read_zip_json(jar, path)
        if error:
            asset_issues.append(error)
            continue
        if data:
            issues, warnings = inspect_model_file(path, data, names, asset_namespaces)
            asset_issues.extend(issues)
            asset_warnings.extend(warnings)

    for path in blockstate_paths:
        data, error = read_zip_json(jar, path)
        if error:
            asset_issues.append(error)
            continue
        if data:
            issues, warnings = inspect_blockstate_file(path, data, names, asset_namespaces)
            asset_issues.extend(issues)
            asset_warnings.extend(warnings)

    for path in recipe_paths:
        data, error = read_zip_json(jar, path)
        if error:
            recipe_issues.append(error)
            continue
        if data:
            issues, warnings, summary = inspect_recipe_file(path, data, names, asset_namespaces)
            recipe_issues.extend(issues)
            recipe_warnings.extend(warnings)
            recipe_summaries.append(summary)

    return {
        "assets": {
            "namespaces": sorted(asset_namespaces),
            "models": len(model_paths),
            "blockstates": len(blockstate_paths),
            "textures": len(texture_paths),
            "issues": asset_issues[:200],
            "warnings": asset_warnings[:200],
        },
        "recipes": {
            "namespaces": sorted(data_namespaces),
            "recipes": len(recipe_paths),
            "craftingRecipes": sum(1 for item in recipe_summaries if str(item.get("type", "")).startswith("minecraft:crafting_")),
            "items": recipe_summaries[:200],
            "issues": recipe_issues[:200],
            "warnings": recipe_warnings[:200],
        },
    }


def inspect_mod(jar_path: str, profile: dict[str, Any] | None = None) -> dict[str, Any]:
    path = Path(jar_path).expanduser()
    result: dict[str, Any] = {
        "path": str(path),
        "fileName": path.name,
        "enabled": not path.name.lower().endswith(".disabled"),
        "exists": path.exists(),
        "readable": False,
        "size": 0,
        "sha1": "",
        "loaders": [],
        "metadata": [],
        "content": {},
        "issues": [],
        "warnings": [],
        "compatibility": [],
    }

    if not path.exists():
        result["issues"].append("mod file does not exist")
        return result
    if not path.is_file():
        result["issues"].append("mod path is not a file")
        return result

    result["size"] = path.stat().st_size
    try:
        result["sha1"] = sha1_file(path)
    except Exception as exc:
        result["warnings"].append(f"cannot calculate sha1: {exc}")

    try:
        with zipfile.ZipFile(path) as jar:
            names = set(jar.namelist())
            result["readable"] = True

            fabric, error = read_zip_json(jar, "fabric.mod.json")
            if error:
                result["issues"].append(error)
            if fabric:
                result["loaders"].append("fabric")
                result["metadata"].append(inspect_fabric_metadata("fabric.mod.json", fabric, names))
                nested_metadata, nested_issues = inspect_nested_fabric_jars(jar, "fabric.mod.json", fabric, names)
                for nested in nested_metadata:
                    result["loaders"].append("fabric")
                    result["metadata"].append(nested)
                result["issues"].extend(nested_issues)

            quilt, error = read_zip_json(jar, "quilt.mod.json")
            if error:
                result["issues"].append(error)
            if quilt:
                result["loaders"].append("quilt")
                result["metadata"].append(inspect_quilt_metadata("quilt.mod.json", quilt, names))

            for metadata_file, loader in (
                ("META-INF/mods.toml", "forge"),
                ("META-INF/neoforge.mods.toml", "neoforge"),
            ):
                text, error = read_zip_text(jar, metadata_file)
                if error:
                    result["issues"].append(error)
                if text:
                    result["loaders"].append(loader)
                    result["metadata"].append(inspect_forge_metadata(metadata_file, text, loader))

            legacy_forge_text, error = read_zip_text(jar, "mcmod.info")
            if error:
                result["issues"].append(error)
            if legacy_forge_text:
                try:
                    legacy_forge = json.loads(legacy_forge_text)
                    result["loaders"].append("forge")
                    result["metadata"].append(inspect_legacy_forge_metadata("mcmod.info", legacy_forge))
                except json.JSONDecodeError as exc:
                    result["issues"].append(f"cannot parse mcmod.info: {exc}")

            if "META-INF/MANIFEST.MF" not in names:
                result["warnings"].append("jar has no META-INF/MANIFEST.MF")
            result["content"] = inspect_mod_content(jar, names)
    except zipfile.BadZipFile:
        result["issues"].append("file is not a readable jar/zip archive")
        return result
    except Exception as exc:
        result["issues"].append(f"cannot inspect jar: {exc}")
        return result

    if not result["loaders"]:
        result["issues"].append("no recognized Fabric, Quilt, Forge, or NeoForge metadata found")
    if len(set(result["loaders"])) > 1:
        result["warnings"].append("jar declares multiple loader metadata files")
    result["loaders"] = sorted(set(result["loaders"]))

    for metadata in result["metadata"]:
        result["issues"].extend(metadata.get("issues", []))
        result["warnings"].extend(metadata.get("warnings", []))
    for section in ("assets", "recipes"):
        content_section = result.get("content", {}).get(section, {})
        result["issues"].extend(content_section.get("issues", []))
        result["warnings"].extend(content_section.get("warnings", []))

    if profile:
        result["compatibility"] = compatibility_with_profile(result, profile)
        for item in result["compatibility"]:
            if item.get("severity") == "error":
                result["issues"].append(item["message"])
            elif item.get("severity") == "warning":
                result["warnings"].append(item["message"])
    return result


def diagnose_mod_content(jar_path: str) -> dict[str, Any]:
    inspection = inspect_mod(jar_path)
    content = inspection.get("content") or {}
    asset_issues = content.get("assets", {}).get("issues", [])
    asset_warnings = content.get("assets", {}).get("warnings", [])
    recipe_issues = content.get("recipes", {}).get("issues", [])
    recipe_warnings = content.get("recipes", {}).get("warnings", [])
    issues = list(asset_issues) + list(recipe_issues)
    warnings = list(asset_warnings) + list(recipe_warnings)
    return {
        "status": "error" if issues else "warning" if warnings else "ok",
        "path": inspection.get("path", str(Path(jar_path).expanduser())),
        "fileName": inspection.get("fileName", Path(jar_path).name),
        "readable": inspection.get("readable", False),
        "summary": {
            "issues": len(issues),
            "warnings": len(warnings),
            "assetModels": content.get("assets", {}).get("models", 0),
            "assetBlockstates": content.get("assets", {}).get("blockstates", 0),
            "assetTextures": content.get("assets", {}).get("textures", 0),
            "recipes": content.get("recipes", {}).get("recipes", 0),
            "craftingRecipes": content.get("recipes", {}).get("craftingRecipes", 0),
        },
        "content": content,
        "issues": issues,
        "warnings": warnings,
    }


def version_parts(value: str) -> list[int]:
    return [int(part) for part in re.findall(r"\d+", value)[:4]]


def compare_versions(left: str, right: str) -> int:
    a = version_parts(left)
    b = version_parts(right)
    width = max(len(a), len(b), 1)
    a += [0] * (width - len(a))
    b += [0] * (width - len(b))
    return (a > b) - (a < b)


def matches_single_constraint(version: str, constraint: str) -> bool | None:
    constraint = constraint.strip()
    if not constraint or constraint == "*":
        return True
    if "||" in constraint:
        values = [matches_constraint(version, item) for item in constraint.split("||")]
        return True if True in values else None if None in values else False
    if constraint.endswith(".x"):
        return version.startswith(constraint[:-1])
    if constraint.startswith(">="):
        return compare_versions(version, constraint[2:].strip()) >= 0
    if constraint.startswith("<="):
        return compare_versions(version, constraint[2:].strip()) <= 0
    if constraint.startswith(">"):
        return compare_versions(version, constraint[1:].strip()) > 0
    if constraint.startswith("<"):
        return compare_versions(version, constraint[1:].strip()) < 0
    if constraint.startswith("="):
        return compare_versions(version, constraint[1:].strip()) == 0
    if constraint.startswith("~"):
        base = constraint[1:].strip()
        parts = version_parts(base)
        if len(parts) >= 2:
            prefix = ".".join(str(part) for part in parts[:2]) + "."
            return version == base or version.startswith(prefix)
        return None
    if constraint.startswith("[") or constraint.startswith("("):
        return matches_maven_range(version, constraint)
    if re.match(r"^\d+(\.\d+)*$", constraint):
        return compare_versions(version, constraint) == 0
    return None


def matches_maven_range(version: str, constraint: str) -> bool | None:
    if re.match(r"^\[[^,\]]+\]$", constraint.strip()):
        return compare_versions(version, constraint.strip()[1:-1]) == 0
    match = re.match(r"^([\[(])([^,]*),?([^\])]*)([])])$", constraint.strip())
    if not match:
        return None
    lower_inclusive = match.group(1) == "["
    upper_inclusive = match.group(4) == "]"
    lower = match.group(2).strip()
    upper = match.group(3).strip()
    if lower:
        cmp_lower = compare_versions(version, lower)
        if cmp_lower < 0 or (cmp_lower == 0 and not lower_inclusive):
            return False
    if upper:
        cmp_upper = compare_versions(version, upper)
        if cmp_upper > 0 or (cmp_upper == 0 and not upper_inclusive):
            return False
    return True


def matches_constraint(version: str, raw_constraint: Any) -> bool | None:
    if isinstance(raw_constraint, list):
        values = [matches_constraint(version, item) for item in raw_constraint]
        return True if True in values else None if None in values else False
    if isinstance(raw_constraint, dict):
        raw_constraint = raw_constraint.get("version") or raw_constraint.get("versions")
    if not isinstance(raw_constraint, str):
        return None
    raw_constraint = raw_constraint.strip()
    if raw_constraint.startswith(("[", "(")) and raw_constraint.endswith(("]", ")")):
        return matches_single_constraint(version, raw_constraint)
    parts = [part for part in re.split(r"(?<![<>=])\s+|,\s*", raw_constraint) if part.strip()]
    if not parts:
        return True
    results = [matches_single_constraint(version, part) for part in parts]
    if False in results:
        return False
    if None in results:
        return None
    return True


def required_java_for_minecraft(minecraft_version: str) -> int | None:
    if not version_parts(minecraft_version):
        return None
    if compare_versions(minecraft_version, "1.20.5") >= 0:
        return 21
    if compare_versions(minecraft_version, "1.18") >= 0:
        return 17
    return 8


def dependency_value(metadata: dict[str, Any], *names: str) -> Any:
    depends = metadata.get("depends", {})
    if isinstance(depends, dict):
        for section in ("depends", "breaks", "recommends", "suggests"):
            values = depends.get(section)
            if not isinstance(values, dict):
                continue
            for name in names:
                if name in values:
                    return values[name]
    return None


def forge_dependency_value(metadata: dict[str, Any], mod_id: str) -> Any:
    for item in metadata.get("dependencies", []):
        if not isinstance(item, dict):
            continue
        if item.get("modId") == mod_id:
            return item.get("versionRange")
    return None


def compatibility_with_profile(inspection: dict[str, Any], profile: dict[str, Any]) -> list[dict[str, str]]:
    loader = ((profile.get("loader") or {}).get("type") or "vanilla").lower()
    minecraft_version = str(profile.get("minecraftVersion", ""))
    found = set(inspection.get("loaders", []))
    items: list[dict[str, str]] = []

    if not found:
        return items
    if loader == "vanilla":
        items.append({"severity": "error", "message": "profile is vanilla but the jar is a loader mod"})
    elif loader == "fabric" and "fabric" not in found:
        items.append({"severity": "error", "message": f"profile loader is Fabric but jar declares {', '.join(sorted(found))}"})
    elif loader == "quilt" and not (found & {"quilt", "fabric"}):
        items.append({"severity": "error", "message": f"profile loader is Quilt but jar declares {', '.join(sorted(found))}"})
    elif loader == "forge" and "forge" not in found:
        items.append({"severity": "error", "message": f"profile loader is Forge but jar declares {', '.join(sorted(found))}"})
    elif loader == "neoforge" and "neoforge" not in found:
        items.append({"severity": "error", "message": f"profile loader is NeoForge but jar declares {', '.join(sorted(found))}"})
    if loader == "quilt" and found == {"fabric"}:
        items.append({"severity": "warning", "message": "Fabric mod in a Quilt profile may need Quilt Standard Libraries or QFAPI"})

    for metadata in inspection.get("metadata", []):
        mc_constraint = dependency_value(metadata, "minecraft") or forge_dependency_value(metadata, "minecraft")
        if mc_constraint:
            match = matches_constraint(minecraft_version, mc_constraint)
            if match is False:
                items.append({
                    "severity": "error",
                    "message": f"mod declares Minecraft constraint {mc_constraint!r}, profile uses {minecraft_version}",
                })
            elif match is None:
                items.append({
                    "severity": "warning",
                    "message": f"cannot verify Minecraft constraint {mc_constraint!r} against {minecraft_version}",
                })

        java_constraint = dependency_value(metadata, "java")
        if java_constraint:
            required = required_java_for_minecraft(minecraft_version)
            match = matches_constraint(str(required or ""), java_constraint)
            if required and match is False:
                items.append({
                    "severity": "warning",
                    "message": f"mod declares Java constraint {java_constraint!r}; Minecraft {minecraft_version} normally uses Java {required}",
                })
            elif match is None:
                items.append({"severity": "warning", "message": f"cannot verify Java constraint {java_constraint!r}"})
    return items


def profile_loader_version(profile: dict[str, Any]) -> str:
    loader = profile.get("loader") if isinstance(profile.get("loader"), dict) else {}
    version = str(loader.get("version", "") or "")
    if version and version.lower() != "latest":
        return version

    install = profile.get("install") if isinstance(profile.get("install"), dict) else {}
    message = str(install.get("message", "") or "")
    match = re.search(r"Installed\s+(?:Fabric|Quilt|Forge|NeoForge)\s+loader\s+([^\s]+)", message, re.I)
    if match:
        return match.group(1)
    match = re.search(r"\bloader\s+([0-9][0-9A-Za-z_.+-]*)\b", message, re.I)
    return match.group(1) if match else ""


def installed_mod_index(profile: dict[str, Any], mods: list[dict[str, Any]]) -> dict[str, list[dict[str, str]]]:
    index: dict[str, list[dict[str, str]]] = {}

    def add(mod_id: str, version: str, source: str) -> None:
        mod_id = mod_id.strip()
        if not mod_id:
            return
        index.setdefault(mod_id, []).append({"id": mod_id, "version": str(version or ""), "source": source})

    minecraft_version = str(profile.get("minecraftVersion", "") or "")
    add("minecraft", minecraft_version, "profile")
    required_java = required_java_for_minecraft(minecraft_version)
    if required_java:
        add("java", str(required_java), "profile")

    loader = ((profile.get("loader") or {}).get("type") or "vanilla").lower()
    loader_version = profile_loader_version(profile)
    if loader == "fabric":
        add("fabricloader", loader_version, "profile")
    elif loader == "quilt":
        add("quilt_loader", loader_version, "profile")

    for mod in mods:
        if not mod.get("enabled", True):
            continue
        for metadata in mod.get("metadata", []):
            mod_id = str(metadata.get("id", "") or "")
            version = str(metadata.get("version", "") or "")
            add(mod_id, version, str(mod.get("fileName", "") or mod.get("path", "")))
    return index


def fabric_dependency_entries(metadata: dict[str, Any], section: str) -> list[tuple[str, Any]]:
    depends = metadata.get("depends")
    if not isinstance(depends, dict):
        return []
    values = depends.get(section)
    if not isinstance(values, dict):
        return []
    return [(str(mod_id), constraint) for mod_id, constraint in values.items()]


def constraint_label(value: Any) -> str:
    if isinstance(value, str):
        return value
    return json.dumps(value, ensure_ascii=False, sort_keys=True)


def installed_entries_match_constraint(entries: list[dict[str, str]], constraint: Any) -> bool | None:
    if isinstance(constraint, str) and constraint.strip() in {"", "*"}:
        return True
    unknown = False
    for entry in entries:
        version = str(entry.get("version", "") or "")
        if not version or version.startswith("${"):
            unknown = True
            continue
        match = matches_constraint(version, constraint)
        if match is True:
            return True
        if match is None:
            unknown = True
    return None if unknown else False


def add_profile_dependency_diagnostics(profile: dict[str, Any], mods: list[dict[str, Any]]) -> None:
    index = installed_mod_index(profile, mods)
    for mod in mods:
        if not mod.get("enabled", True):
            continue
        for metadata in mod.get("metadata", []):
            if metadata.get("loader") != "fabric":
                continue
            owner_id = str(metadata.get("id", "") or mod.get("fileName", "mod"))
            for dep_id, constraint in fabric_dependency_entries(metadata, "depends"):
                if dep_id in PROFILE_PROVIDED_MOD_IDS:
                    continue
                entries = index.get(dep_id, [])
                label = constraint_label(constraint)
                if not entries:
                    mod["issues"].append(f"missing required Fabric dependency: {dep_id} required by {owner_id} ({label})")
                    continue
                match = installed_entries_match_constraint(entries, constraint)
                if match is False:
                    installed = ", ".join(entry.get("version", "") or "unknown" for entry in entries)
                    mod["issues"].append(
                        f"Fabric dependency version mismatch: {owner_id} requires {dep_id} {label}, installed {installed}"
                    )
                elif match is None:
                    mod["warnings"].append(f"cannot verify Fabric dependency {dep_id} {label} required by {owner_id}")

            for dep_id, constraint in fabric_dependency_entries(metadata, "breaks"):
                if dep_id in PROFILE_PROVIDED_MOD_IDS:
                    continue
                entries = index.get(dep_id, [])
                if not entries:
                    continue
                match = installed_entries_match_constraint(entries, constraint)
                label = constraint_label(constraint)
                if match is True:
                    installed = ", ".join(entry.get("version", "") or "unknown" for entry in entries)
                    mod["issues"].append(f"Fabric breakage conflict: {owner_id} breaks {dep_id} {label}, installed {installed}")
                elif match is None:
                    mod["warnings"].append(f"cannot verify Fabric breakage rule: {owner_id} breaks {dep_id} {label}")


def list_profile_mod_paths(profile: dict[str, Any]) -> list[Path]:
    game_dir = Path(str(profile.get("gameDir", ""))).expanduser()
    mods_dir = game_dir / "mods"
    if not mods_dir.is_dir():
        return []
    return sorted(
        [
            path
            for path in mods_dir.iterdir()
            if path.is_file() and (path.name.lower().endswith(".jar") or path.name.lower().endswith(".jar.disabled"))
        ],
        key=lambda item: item.name.lower(),
    )


def profile_mods_dir(profile: dict[str, Any], create: bool = False) -> Path:
    game_dir = Path(str(profile.get("gameDir", ""))).expanduser()
    if not str(game_dir).strip():
        raise ValueError("profile gameDir is empty")
    mods_dir = game_dir / "mods"
    if create:
        mods_dir.mkdir(parents=True, exist_ok=True)
    return mods_dir


def clean_mod_file_name(file_name: str) -> str:
    name = Path(str(file_name).strip()).name
    if not name:
        raise ValueError("mod file name is required")
    lower = name.lower()
    if "/" in name or "\\" in name or name in {".", ".."}:
        raise ValueError(f"invalid mod file name: {file_name}")
    if not (lower.endswith(".jar") or lower.endswith(".jar.disabled")):
        raise ValueError("mod file name must end with .jar or .jar.disabled")
    return name


def enabled_mod_name(file_name: str) -> str:
    name = clean_mod_file_name(file_name)
    if name.lower().endswith(".jar.disabled"):
        return name[:-len(".disabled")]
    return name


def disabled_mod_name(file_name: str) -> str:
    name = enabled_mod_name(file_name)
    return name + ".disabled"


def find_existing_mod_path(profile: dict[str, Any], file_name: str) -> Path:
    mods_dir = profile_mods_dir(profile)
    candidates = [mods_dir / clean_mod_file_name(file_name)]
    enabled = enabled_mod_name(file_name)
    candidates.extend([mods_dir / enabled, mods_dir / (enabled + ".disabled")])
    seen: set[Path] = set()
    for path in candidates:
        if path in seen:
            continue
        seen.add(path)
        if path.is_file():
            return path
    raise FileNotFoundError(f"mod file not found: {file_name}")


def mod_file_summary(path: Path, profile: dict[str, Any] | None = None) -> dict[str, Any]:
    inspected = inspect_mod(str(path), profile)
    return {
        "fileName": path.name,
        "path": str(path),
        "enabled": not path.name.lower().endswith(".disabled"),
        "size": inspected.get("size", 0),
        "sha1": inspected.get("sha1", ""),
        "loaders": inspected.get("loaders", []),
        "issues": inspected.get("issues", []),
        "warnings": inspected.get("warnings", []),
    }


def import_profile_mod(
    ctx: DiagnosticContext,
    profile_id: str | None,
    jar_path: str,
    file_name: str | None = None,
    enabled: bool = True,
    replace: bool = False,
) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    source = Path(jar_path).expanduser()
    if not source.is_file():
        raise FileNotFoundError(f"mod jar not found: {source}")
    source_name = clean_mod_file_name(source.name)
    target_name = clean_mod_file_name(file_name) if file_name else source_name
    target_name = enabled_mod_name(target_name) if enabled else disabled_mod_name(target_name)

    inspection = inspect_mod(str(source), profile)
    if not inspection.get("readable"):
        raise ValueError("source is not a readable mod jar")

    mods_dir = profile_mods_dir(profile, create=True)
    target = mods_dir / target_name
    counterpart = mods_dir / (disabled_mod_name(target_name) if enabled else enabled_mod_name(target_name))
    if not replace and (target.exists() or counterpart.exists()):
        raise FileExistsError(f"mod already exists in profile: {target_name}")
    if target.exists() and target.is_dir():
        raise IsADirectoryError(str(target))

    temp = target.with_name("." + target.name + ".power-mine-tmp")
    try:
        shutil.copy2(source, temp)
        temp.chmod(0o644)
        temp.replace(target)
        if replace and counterpart.exists() and counterpart != target:
            counterpart.unlink()
    finally:
        if temp.exists():
            temp.unlink()

    return {
        "profile": profile_brief(profile),
        "imported": mod_file_summary(target, profile),
        "modList": [mod_file_summary(path, profile) for path in list_profile_mod_paths(profile)],
    }


def set_profile_mod_enabled(ctx: DiagnosticContext, profile_id: str | None, file_name: str, enabled: bool) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    current = find_existing_mod_path(profile, file_name)
    target = profile_mods_dir(profile) / (enabled_mod_name(current.name) if enabled else disabled_mod_name(current.name))
    if current == target:
        changed = False
    else:
        if target.exists():
            raise FileExistsError(f"target mod file already exists: {target.name}")
        current.rename(target)
        changed = True
    return {
        "profile": profile_brief(profile),
        "changed": changed,
        "mod": mod_file_summary(target, profile),
        "modList": [mod_file_summary(path, profile) for path in list_profile_mod_paths(profile)],
    }


def delete_profile_mod(ctx: DiagnosticContext, profile_id: str | None, file_name: str) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    current = find_existing_mod_path(profile, file_name)
    summary = mod_file_summary(current, profile)
    current.unlink()
    return {
        "profile": profile_brief(profile),
        "deleted": summary,
        "modList": [mod_file_summary(path, profile) for path in list_profile_mod_paths(profile)],
    }


def repo_root() -> Path:
    return Path(os.environ.get("POWER_MINE_REPO", Path(__file__).resolve().parents[2])).expanduser()


def find_power_mine_binary() -> list[str]:
    configured = os.environ.get("POWER_MINE_BINARY")
    if configured:
        return [configured]
    repo = repo_root()
    candidates = [
        repo / "build" / "bin" / "power-mine",
    ]
    for candidate in candidates:
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return [str(candidate)]
    if shutil.which("go") and (repo / "go.mod").is_file():
        return power_mine_go_run_command()
    appimage = repo / "dist" / "power-mine-0.1.0-linux-x86_64.appimage"
    if appimage.is_file() and os.access(appimage, os.X_OK):
        return [str(appimage)]
    raise FileNotFoundError("Power Mine binary not found; set POWER_MINE_BINARY or build the launcher")


def power_mine_go_run_command() -> list[str]:
    tags = os.environ.get("POWER_MINE_GO_TAGS", "").strip()
    if not tags:
        tags = "desktop,production"
        if sys.platform.startswith("linux") and pkg_config_exists("webkit2gtk-4.1"):
            tags += ",webkit2_41"
    return ["go", "run", "-tags", tags, "."]


def pkg_config_exists(package: str) -> bool:
    if not shutil.which("pkg-config"):
        return False
    return (
        subprocess.run(
            ["pkg-config", "--exists", package],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        ).returncode
        == 0
    )


def run_power_mine_headless_args(ctx: DiagnosticContext, command: str, args: list[str]) -> dict[str, Any]:
    repo = repo_root()
    invocation = find_power_mine_binary() + [
        "headless",
        command,
        "--data-dir",
        str(ctx.data_dir),
        *args,
    ]
    completed = subprocess.run(
        invocation,
        cwd=repo,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    try:
        payload = json.loads(completed.stdout)
    except json.JSONDecodeError:
        payload = {"ok": False, "command": command, "error": completed.stdout.strip()}
    payload["exitCode"] = completed.returncode
    if completed.stderr.strip():
        payload["stderr"] = completed.stderr.strip()
    if completed.returncode != 0 and not payload.get("error"):
        payload["error"] = completed.stderr.strip() or f"headless command failed: {command}"
    return payload


def create_profile(
    ctx: DiagnosticContext,
    name: str,
    minecraft_version: str = "1.20.1",
    loader: str = "fabric",
    loader_version: str = "latest",
    game_dir: str | None = None,
    min_memory: int = 1024,
    max_memory: int = 4096,
    install: bool = False,
) -> dict[str, Any]:
    args = [
        "--name",
        name,
        "--minecraft-version",
        minecraft_version,
        "--loader",
        loader,
        "--loader-version",
        loader_version,
        "--min-memory",
        str(min_memory),
        "--max-memory",
        str(max_memory),
    ]
    if game_dir:
        args.extend(["--game-dir", game_dir])
    if install:
        args.append("--install")
    return run_power_mine_headless_args(ctx, "create-profile", args)


def install_java_runtime(ctx: DiagnosticContext, version: int = 21) -> dict[str, Any]:
    return run_power_mine_headless_args(ctx, "install-java", ["--version", str(version)])


def run_power_mine_headless(ctx: DiagnosticContext, command: str, profile_id: str | None) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    return run_power_mine_headless_args(ctx, command, ["--profile-id", profile["id"]])


def launch_profile(
    ctx: DiagnosticContext,
    profile_id: str | None,
    quick_play_singleplayer: str | None = None,
    keep_pause_on_lost_focus: bool = False,
) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    args = ["--profile-id", profile["id"]]
    if quick_play_singleplayer:
        args.extend(["--quick-play-singleplayer", quick_play_singleplayer])
    if keep_pause_on_lost_focus:
        args.append("--keep-pause-on-lost-focus")
    return run_power_mine_headless_args(ctx, "launch-profile", args)


def normalize_agent_target(target: str | None) -> str:
    target = (target or FABRIC_AGENT_TARGET).strip().lower()
    aliases = {
        "fabric": FABRIC_AGENT_TARGET,
        "fabric-1.20.1": FABRIC_AGENT_TARGET,
        "forge": FORGE_1710_AGENT_TARGET,
        "forge-1.7.10": FORGE_1710_AGENT_TARGET,
        "1.7.10-forge": FORGE_1710_AGENT_TARGET,
        "forge-1.12.2": FORGE_1122_AGENT_TARGET,
        "forge-1122": FORGE_1122_AGENT_TARGET,
        "1.12.2-forge": FORGE_1122_AGENT_TARGET,
    }
    normalized = aliases.get(target, target)
    if normalized not in AGENT_TARGETS:
        raise ValueError(f"unsupported agent target: {target}")
    return normalized


def agent_target_for_profile(profile: dict[str, Any]) -> str:
    loader = ((profile.get("loader") or {}).get("type") or "vanilla").strip().lower()
    minecraft_version = str(profile.get("minecraftVersion", "")).strip()
    if loader == "fabric" and minecraft_version == "1.20.1":
        return FABRIC_AGENT_TARGET
    if loader == "forge" and minecraft_version == "1.7.10":
        return FORGE_1710_AGENT_TARGET
    if loader == "forge" and minecraft_version == "1.12.2":
        return FORGE_1122_AGENT_TARGET
    raise ValueError(
        f"no runtime agent is available for loader={loader or 'vanilla'} minecraft={minecraft_version or 'unknown'}"
    )


def build_agent_mod(target: str | None = None) -> dict[str, Any]:
    target = normalize_agent_target(target)
    repo = repo_root()
    script = repo / "scripts" / "build-agent.sh"
    if not script.is_file():
        raise FileNotFoundError(f"agent build script not found: {script}")
    completed = subprocess.run(
        [str(script), target],
        cwd=repo,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    jar = latest_agent_jar(target)
    return {
        "ok": completed.returncode == 0,
        "command": "build-agent",
        "target": target,
        "exitCode": completed.returncode,
        "jarPath": str(jar) if jar else "",
        "stdout": completed.stdout.strip()[-4000:],
        "stderr": completed.stderr.strip()[-4000:],
        "error": "" if completed.returncode == 0 else "agent build failed",
    }


def latest_agent_jar(target: str | None = None) -> Path | None:
    target = normalize_agent_target(target)
    config = AGENT_TARGETS[target]
    libs = repo_root().joinpath(*config["libs"])
    if not libs.is_dir():
        return None
    jars = [
        path
        for path in libs.iterdir()
        if path.is_file() and path.suffix == ".jar" and "-sources" not in path.name and "-dev" not in path.name
    ]
    if not jars:
        return None
    return sorted(jars, key=lambda item: item.stat().st_mtime, reverse=True)[0]


def install_agent_mod(
    ctx: DiagnosticContext,
    profile_id: str | None,
    build: bool = True,
    replace: bool = True,
    target: str | None = None,
) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    target = normalize_agent_target(target) if target else agent_target_for_profile(profile)
    config = AGENT_TARGETS[target]
    if build or latest_agent_jar(target) is None:
        build_result = build_agent_mod(target)
        if not build_result.get("ok"):
            return build_result
    jar = latest_agent_jar(target)
    if jar is None:
        raise FileNotFoundError(f"Power Mine agent jar not found for {target}; run build-agent first")
    result = import_profile_mod(ctx, profile["id"], str(jar), str(config["fileName"]), True, replace)
    result["agent"] = {
        "jarPath": str(jar),
        "target": target,
        "description": config["description"],
        "fileName": config["fileName"],
        "port": DEFAULT_AGENT_PORT,
    }
    return result


def resolve_project_build_command(project: Path, build_command: str | None = None) -> tuple[list[str], str]:
    if build_command:
        command = shlex.split(build_command)
        if not command:
            raise ValueError("build_command is empty")
        return command, "custom"

    forge_script = project / "scripts" / "build-forge-1.7.10.sh"
    if forge_script.is_file():
        return ["bash", str(forge_script)], "forge-1.7.10-script"
    if (project / "gradlew").is_file():
        return ["./gradlew", "build"], "gradle-wrapper"
    if (project / "build.gradle").is_file() or (project / "build.gradle.kts").is_file():
        return ["gradle", "build"], "gradle"
    if (project / "pom.xml").is_file():
        return ["mvn", "package"], "maven"
    raise FileNotFoundError("no supported build file found; pass build_command")


def candidate_mod_artifacts(project: Path, artifact_glob: str | None = None) -> list[Path]:
    if artifact_glob:
        candidates = list(project.glob(artifact_glob))
    else:
        candidates = []
        for pattern in ("build/libs/*.jar", "target/*.jar", "build/*.jar", "dist/*.jar"):
            candidates.extend(project.glob(pattern))

    def usable(path: Path) -> bool:
        if not path.is_file() or path.suffix.lower() != ".jar":
            return False
        lowered = path.name.lower()
        blocked = ("-sources", "-javadoc", "-dev", "-dev-shadow", "original-")
        return not any(marker in lowered for marker in blocked)

    return sorted(
        [path for path in candidates if usable(path)],
        key=lambda item: (item.stat().st_mtime, item.stat().st_size),
        reverse=True,
    )


def build_mod_project(
    project_dir: str,
    build_command: str | None = None,
    artifact_glob: str | None = None,
    timeout_seconds: int = 300,
) -> dict[str, Any]:
    project = Path(project_dir).expanduser().resolve()
    if not project.is_dir():
        raise FileNotFoundError(f"project directory not found: {project}")

    command, strategy = resolve_project_build_command(project, build_command)
    started_at = time.time()
    try:
        completed = subprocess.run(
            command,
            cwd=project,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=max(1, int(timeout_seconds)),
            check=False,
        )
        exit_code = completed.returncode
        error = "" if exit_code == 0 else "mod project build failed"
    except FileNotFoundError as exc:
        completed = None
        exit_code = 127
        error = str(exc)
        stdout = ""
        stderr = ""
    except subprocess.TimeoutExpired as exc:
        completed = None
        exit_code = 124
        error = f"mod project build timed out after {timeout_seconds}s"
        stdout = exc.stdout.decode("utf-8", errors="replace") if isinstance(exc.stdout, bytes) else (exc.stdout or "")
        stderr = exc.stderr.decode("utf-8", errors="replace") if isinstance(exc.stderr, bytes) else (exc.stderr or "")
    else:
        stdout = completed.stdout
        stderr = completed.stderr

    artifacts = candidate_mod_artifacts(project, artifact_glob)
    jar = artifacts[0] if artifacts else None
    return {
        "ok": exit_code == 0 and jar is not None,
        "command": command,
        "commandText": shlex.join(command),
        "strategy": strategy,
        "projectDir": str(project),
        "exitCode": exit_code,
        "durationSeconds": round(time.time() - started_at, 3),
        "jarPath": str(jar) if jar else "",
        "artifactCandidates": [str(path) for path in artifacts[:20]],
        "stdout": stdout.strip()[-8000:],
        "stderr": stderr.strip()[-8000:],
        "error": "" if exit_code == 0 and jar is not None else error or "built jar artifact not found",
    }


def find_built_mod_project_jar(project_dir: str, artifact_glob: str | None = None) -> dict[str, Any]:
    project = Path(project_dir).expanduser().resolve()
    if not project.is_dir():
        raise FileNotFoundError(f"project directory not found: {project}")
    artifacts = candidate_mod_artifacts(project, artifact_glob)
    jar = artifacts[0] if artifacts else None
    return {
        "ok": jar is not None,
        "projectDir": str(project),
        "jarPath": str(jar) if jar else "",
        "artifactCandidates": [str(path) for path in artifacts[:20]],
        "error": "" if jar else "built jar artifact not found",
    }


def first_declared_mod_id(inspection: dict[str, Any]) -> str:
    for metadata in inspection.get("metadata", []):
        if not isinstance(metadata, dict):
            continue
        mod_id = str(metadata.get("id", "") or "").strip()
        if mod_id and mod_id not in PROFILE_PROVIDED_MOD_IDS:
            return mod_id
    return ""


def extract_launch_pid_from_smoke(smoke: dict[str, Any]) -> int | None:
    for step in smoke.get("steps", []):
        if not isinstance(step, dict) or step.get("name") != "launch_profile":
            continue
        data = step.get("data")
        result = data.get("result") if isinstance(data, dict) else None
        pid = result.get("pid") if isinstance(result, dict) else None
        if isinstance(pid, int) and pid > 0:
            return pid
    return None


def terminate_process(pid: int, wait_seconds: float = 5.0) -> dict[str, Any]:
    result: dict[str, Any] = {"ok": False, "pid": pid, "terminated": False}
    try:
        os.kill(pid, signal.SIGTERM)
        result["signal"] = "SIGTERM"
    except ProcessLookupError:
        result["ok"] = True
        result["terminated"] = True
        result["alreadyExited"] = True
        return result
    except Exception as exc:
        result["error"] = str(exc)
        return result

    deadline = time.time() + max(0.25, wait_seconds)
    while time.time() < deadline:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            result["ok"] = True
            result["terminated"] = True
            return result
        time.sleep(0.25)

    result["ok"] = True
    result["terminated"] = False
    result["warning"] = "process still exists after SIGTERM wait"
    return result


def agent_http_request(
    endpoint: str,
    method: str = "GET",
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
    body: dict[str, Any] | None = None,
    query: dict[str, Any] | None = None,
    timeout_seconds: int = 10,
) -> dict[str, Any]:
    if not endpoint.startswith("/"):
        endpoint = "/" + endpoint
    query_string = ""
    if query:
        clean_query = {key: value for key, value in query.items() if value is not None}
        query_string = "?" + urllib.parse.urlencode(clean_query) if clean_query else ""
    url = f"http://127.0.0.1:{port}{endpoint}{query_string}"
    raw_body = None
    headers = {"Accept": "application/json"}
    if body is not None:
        raw_body = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    token = token or os.environ.get("POWER_MINE_AGENT_TOKEN")
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(url, data=raw_body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=max(1, int(timeout_seconds))) as response:
            raw = response.read().decode("utf-8")
            data = json.loads(raw) if raw else {}
            if isinstance(data, dict):
                data.setdefault("httpStatus", response.status)
                data.setdefault("url", url)
                return data
            return {"ok": True, "httpStatus": response.status, "url": url, "data": data}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            data = json.loads(raw)
            if isinstance(data, dict):
                data.setdefault("httpStatus", exc.code)
                data.setdefault("url", url)
                return data
        except json.JSONDecodeError:
            pass
        return {"ok": False, "httpStatus": exc.code, "url": url, "error": raw or str(exc)}
    except urllib.error.URLError as exc:
        return {"ok": False, "url": url, "error": f"agent is not reachable: {exc.reason}"}


def agent_capabilities(port: int = DEFAULT_AGENT_PORT, token: str | None = None) -> dict[str, Any]:
    data = agent_http_request("/bridge/capabilities", port=port, token=token)
    if data.get("ok") is not False and data.get("capabilities"):
        return data
    fallback = agent_http_request("/capabilities", port=port, token=token)
    if fallback.get("ok") is not False and fallback.get("capabilities"):
        return fallback
    health = agent_http_request("/health", port=port, token=token)
    if health.get("ok") is not False:
        health.setdefault("protocolVersion", health.get("protocolVersion", "legacy"))
        health.setdefault("capabilities", legacy_agent_capabilities())
    return health


def legacy_agent_capabilities() -> dict[str, bool]:
    return {
        "state": True,
        "inventory": True,
        "giveItem": True,
        "selectHotbar": True,
        "worldSnapshot": True,
        "screenshot": True,
        "cameraLook": False,
        "recipeCheck": True,
        "craftRecipe": False,
        "placeBlock": True,
        "breakBlock": True,
        "waitTicks": False,
        "useItem": False,
        "useBlock": False,
        "heldItemRender": True,
        "blockRender": True,
        "bakedModelIntrospection": True,
        "pauseOnLostFocusControl": True,
        "inputRelease": True,
        "offhand": True,
        "autoWorldOpen": False,
        "autoWorldCreate": False,
    }


def agent_supports(capabilities_data: dict[str, Any], key: str, default: bool = True) -> bool:
    capabilities = capabilities_data.get("capabilities")
    if not isinstance(capabilities, dict):
        return default
    value = capabilities.get(key)
    return bool(value) if isinstance(value, bool) else default


def agent_world_open(
    world: str,
    create: bool = True,
    gamemode: str = "creative",
    seed: int | None = None,
    display_name: str | None = None,
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
    timeout_seconds: int = 90,
) -> dict[str, Any]:
    body: dict[str, Any] = {
        "world": world,
        "create": bool(create),
        "gamemode": gamemode,
    }
    if display_name:
        body["displayName"] = display_name
    if seed is not None:
        body["seed"] = int(seed)
    return agent_http_request("/world/open", method="POST", port=port, token=token, body=body, timeout_seconds=timeout_seconds)


def agent_wait_ticks(
    ticks: int = 20,
    timeout_seconds: int = 10,
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
) -> dict[str, Any]:
    timeout_seconds = max(1, int(timeout_seconds))
    return agent_http_request(
        "/tick/wait",
        method="POST",
        port=port,
        token=token,
        body={"ticks": int(ticks), "timeoutSeconds": timeout_seconds},
        timeout_seconds=timeout_seconds + 2,
    )


def agent_use_item(
    hand: str = "main",
    slot: int | None = None,
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {"hand": hand}
    if slot is not None:
        body["slot"] = int(slot)
    return agent_http_request("/item/use", method="POST", port=port, token=token, body=body)


def agent_use_block(
    x: int,
    y: int,
    z: int,
    hand: str = "main",
    side: str = "up",
    slot: int | None = None,
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {
        "x": int(x),
        "y": int(y),
        "z": int(z),
        "hand": hand,
        "side": side,
    }
    if slot is not None:
        body["slot"] = int(slot)
    return agent_http_request("/block/use", method="POST", port=port, token=token, body=body)


def agent_craft_recipe(
    items: list[Any],
    width: int = 3,
    height: int = 3,
    expected_output: str | None = None,
    crafts: int = 1,
    require_inventory: bool = True,
    consume: bool = True,
    insert_output: bool = True,
    output_slot: int | None = None,
    replace_output: bool = False,
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {
        "width": int(width),
        "height": int(height),
        "items": items,
        "expectedOutput": expected_output or "",
        "crafts": int(crafts),
        "requireInventory": bool(require_inventory),
        "consume": bool(consume),
        "insertOutput": bool(insert_output),
        "replaceOutput": bool(replace_output),
    }
    if output_slot is not None:
        body["outputSlot"] = int(output_slot)
    return agent_http_request("/recipe/craft", method="POST", port=port, token=token, body=body)


def recipe_give_specs(items: list[Any], crafts: int = 1) -> list[dict[str, Any]]:
    aggregated: dict[tuple[str, int], int] = {}
    for item in items:
        item_id = ""
        count = 1
        damage = 0
        if isinstance(item, str):
            item_id = item.strip()
        elif isinstance(item, dict):
            item_id = str(item.get("item") or item.get("id") or item.get("block") or "").strip()
            count = int(item.get("count") or 1)
            damage = int(item.get("damage", item.get("metadata", 0)) or 0)
        if not item_id or item_id in {"minecraft:air", "air"}:
            continue
        key = (item_id, damage)
        aggregated[key] = aggregated.get(key, 0) + max(1, count) * max(1, int(crafts))
    return [
        {"item": item_id, "count": count, "damage": damage}
        for (item_id, damage), count in sorted(aggregated.items(), key=lambda entry: entry[0])
    ]


def stack_signature(stack: Any) -> dict[str, Any]:
    if not isinstance(stack, dict):
        return {"empty": True}
    if bool(stack.get("empty", False)):
        return {"empty": True}
    return {
        "empty": False,
        "id": stack.get("id", ""),
        "count": stack.get("count", 0),
        "damage": stack.get("damage", 0),
    }


def block_signature(block: Any) -> dict[str, Any]:
    if not isinstance(block, dict):
        return {}
    return {
        "id": block.get("id", ""),
        "metadata": block.get("metadata"),
        "state": block.get("state", ""),
    }


def selected_inventory_stack(inventory: dict[str, Any], slot: int | None = None) -> dict[str, Any] | None:
    slots = inventory.get("slots")
    if not isinstance(slots, list):
        return None
    selected_slot = int(slot if slot is not None else inventory.get("selectedSlot", 0))
    for entry in slots:
        if isinstance(entry, dict) and int(entry.get("slot", -999)) == selected_slot:
            return entry
    return None


def interaction_bool(value: Any) -> bool | None:
    return value if isinstance(value, bool) else None


def expectation_matches(observed: bool | None, expected: bool | None) -> bool:
    return expected is None or observed is expected


def agent_interaction_check(
    mode: str,
    x: int | None = None,
    y: int | None = None,
    z: int | None = None,
    block: str | None = None,
    item: str | None = None,
    count: int = 1,
    slot: int | None = 0,
    hand: str = "main",
    side: str = "up",
    wait_ticks: int = 2,
    timeout_seconds: int = 10,
    require_effect: bool = False,
    expect_used: bool | None = None,
    expect_screen_open: bool | None = None,
    expect_block_changed: bool | None = None,
    expect_stack_changed: bool | None = None,
    cleanup: bool = False,
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
) -> dict[str, Any]:
    mode = (mode or "").strip().lower()
    if mode not in ("item", "block"):
        raise ValueError("mode must be item or block")

    report: dict[str, Any] = {
        "ok": False,
        "status": "error",
        "mode": mode,
        "config": {
            "x": x,
            "y": y,
            "z": z,
            "block": block or "",
            "item": item or "",
            "count": count,
            "slot": slot,
            "hand": hand,
            "side": side,
            "waitTicks": wait_ticks,
            "timeoutSeconds": timeout_seconds,
            "requireEffect": require_effect,
            "expectUsed": expect_used,
            "expectScreenOpen": expect_screen_open,
            "expectBlockChanged": expect_block_changed,
            "expectStackChanged": expect_stack_changed,
            "cleanup": cleanup,
            "port": port,
        },
        "steps": [],
    }

    capabilities = agent_capabilities(port=port, token=token)
    add_smoke_step(report, "agent_capabilities", http_step_status(capabilities), capabilities)
    if http_step_status(capabilities) == "error":
        return finalize_interaction_report(report)
    capability_key = "useItem" if mode == "item" else "useBlock"
    if not agent_supports(capabilities, capability_key, default=False):
        add_smoke_step(report, "capability_check", "error", data={"reason": f"agent capability {capability_key}=false"})
        return finalize_interaction_report(report)

    state_before = agent_http_request("/state", port=port, token=token)
    add_smoke_step(report, "agent_state_before", http_step_status(state_before), state_before)
    if http_step_status(state_before) == "error" or state_before.get("loaded") is not True:
        return finalize_interaction_report(report)
    screen_before = str(state_before.get("screen") or "")

    inventory_before = agent_http_request("/inventory", port=port, token=token)
    add_smoke_step(report, "agent_inventory_before", http_step_status(inventory_before), inventory_before, required=False)

    selected_slot = slot
    if item:
        if not agent_supports(capabilities, "giveItem", default=False):
            add_smoke_step(report, "agent_give_item", "error", data={"reason": "agent capability giveItem=false"})
            return finalize_interaction_report(report)
        selected_slot = int(slot if slot is not None else 0)
        give_data = agent_http_request(
            "/inventory/give",
            method="POST",
            port=port,
            token=token,
            body={
                "item": item,
                "count": int(count),
                "slot": selected_slot,
                "select": True,
                "replace": True,
            },
        )
        add_smoke_step(report, "agent_give_item", http_step_status(give_data), give_data)
        if http_step_status(give_data) == "error":
            return finalize_interaction_report(report)
    elif selected_slot is not None and agent_supports(capabilities, "selectHotbar", default=False):
        select_data = agent_http_request(
            "/hotbar/select",
            method="POST",
            port=port,
            token=token,
            body={"slot": int(selected_slot)},
        )
        add_smoke_step(report, "agent_select_hotbar_slot", http_step_status(select_data), select_data, required=False)

    pos: tuple[int, int, int] | None = None
    original_block: str | None = None
    before_block: dict[str, Any] | None = None
    if mode == "block":
        if x is None or y is None or z is None:
            x, y, z = default_smoke_block_pos(state_before)
        pos = (int(x), int(y), int(z))
        report["config"]["x"], report["config"]["y"], report["config"]["z"] = pos
        before_probe = agent_block_probe(port, token, pos[0], pos[1], pos[2])
        add_smoke_step(report, "agent_block_before", http_step_status(before_probe), before_probe, required=False)
        original_block = block_probe_id(before_probe) if before_probe.get("ok") is not False else None
        blocks = before_probe.get("blocks")
        if isinstance(blocks, list) and blocks and isinstance(blocks[0], dict):
            before_block = blocks[0]
        elif before_probe.get("id"):
            before_block = before_probe
        if block:
            if not agent_supports(capabilities, "placeBlock", default=False):
                add_smoke_step(report, "agent_place_block", "error", data={"reason": "agent capability placeBlock=false"})
                return finalize_interaction_report(report)
            place_data = agent_http_request(
                "/block/place",
                method="POST",
                port=port,
                token=token,
                body={"x": pos[0], "y": pos[1], "z": pos[2], "block": block},
            )
            place_status = http_step_status(place_data)
            if place_status == "ok" and place_data.get("id") != block:
                place_status = "error"
            add_smoke_step(report, "agent_place_block", place_status, place_data)
            if place_status == "error":
                return finalize_interaction_report(report)

    if mode == "item":
        action_data = agent_use_item(hand=hand, slot=selected_slot, port=port, token=token)
        action_name = "agent_use_item"
    else:
        assert pos is not None
        action_data = agent_use_block(pos[0], pos[1], pos[2], hand=hand, side=side, slot=selected_slot, port=port, token=token)
        action_name = "agent_use_block"
    add_smoke_step(report, action_name, http_step_status(action_data), action_data)
    if http_step_status(action_data) == "error":
        return finalize_interaction_report(report)

    if wait_ticks > 0:
        if agent_supports(capabilities, "waitTicks", default=False):
            wait_data = agent_wait_ticks(wait_ticks, timeout_seconds, port, token)
            wait_status = "ok" if wait_data.get("ok") is not False and wait_data.get("complete") is not False else "error"
            add_smoke_step(report, "agent_wait_ticks", wait_status, wait_data)
        else:
            add_smoke_step(report, "agent_wait_ticks", "skipped", required=False, data={"reason": "agent capability waitTicks=false"})

    state_after = agent_http_request("/state", port=port, token=token)
    add_smoke_step(report, "agent_state_after", http_step_status(state_after), state_after)
    inventory_after = agent_http_request("/inventory", port=port, token=token)
    add_smoke_step(report, "agent_inventory_after", http_step_status(inventory_after), inventory_after, required=False)

    after_block: dict[str, Any] | None = None
    if mode == "block" and pos is not None:
        after_probe = agent_block_probe(port, token, pos[0], pos[1], pos[2])
        add_smoke_step(report, "agent_block_after", http_step_status(after_probe), after_probe, required=False)
        blocks = after_probe.get("blocks")
        if isinstance(blocks, list) and blocks and isinstance(blocks[0], dict):
            after_block = blocks[0]
        elif after_probe.get("id"):
            after_block = after_probe

    used = interaction_bool(action_data.get("used"))
    if used is None:
        used = interaction_bool(action_data.get("accepted"))
    screen_after = str(action_data.get("screen") or state_after.get("screen") or "")
    screen_opened = screen_before == "" and screen_after != ""

    before_stack = action_data.get("beforeStack") or action_data.get("before") or selected_inventory_stack(inventory_before, selected_slot)
    after_stack = action_data.get("afterStack") or action_data.get("after") or selected_inventory_stack(inventory_after, selected_slot)
    stack_changed = stack_signature(before_stack) != stack_signature(after_stack)

    action_before_block = action_data.get("beforeBlock")
    action_after_block = action_data.get("afterBlock")
    block_changed = None
    if mode == "block":
        before_sig = block_signature(action_before_block or before_block)
        after_sig = block_signature(after_block or action_after_block)
        block_changed = bool(before_sig and after_sig and before_sig != after_sig)

    observed_effect = bool(used or screen_opened or stack_changed or block_changed)
    expected = {
        "used": expect_used,
        "screenOpened": expect_screen_open,
        "stackChanged": expect_stack_changed,
        "blockChanged": expect_block_changed,
        "effect": True if require_effect else None,
    }
    expectation_results = {
        "used": expectation_matches(used, expect_used),
        "screenOpened": expectation_matches(screen_opened, expect_screen_open),
        "stackChanged": expectation_matches(stack_changed, expect_stack_changed),
        "blockChanged": expectation_matches(block_changed, expect_block_changed),
        "effect": (not require_effect) or observed_effect,
    }
    report["observed"] = {
        "used": used,
        "screenBefore": screen_before,
        "screenAfter": screen_after,
        "screenOpened": screen_opened,
        "stackChanged": stack_changed,
        "blockChanged": block_changed,
        "effect": observed_effect,
        "beforeStack": before_stack,
        "afterStack": after_stack,
        "beforeBlock": action_before_block or before_block,
        "afterBlock": after_block or action_after_block,
        "actionBeforeBlock": action_before_block,
        "actionAfterBlock": action_after_block,
    }
    report["expected"] = expected
    report["expectationResults"] = expectation_results
    if not all(expectation_results.values()):
        add_smoke_step(
            report,
            "interaction_expectations",
            "error",
            data={"observed": report["observed"], "expected": expected, "expectationResults": expectation_results},
        )
    else:
        add_smoke_step(
            report,
            "interaction_expectations",
            "ok",
            data={"observed": report["observed"], "expected": expected, "expectationResults": expectation_results},
        )

    if cleanup and mode == "block" and pos is not None and block:
        cleanup_data: dict[str, Any]
        if original_block and original_block != "minecraft:air" and original_block != block:
            cleanup_data = agent_http_request(
                "/block/place",
                method="POST",
                port=port,
                token=token,
                body={"x": pos[0], "y": pos[1], "z": pos[2], "block": original_block},
            )
            cleanup_data["warning"] = "restored original block id only; metadata/state restoration is not available"
        elif original_block == block:
            cleanup_data = {"ok": True, "unchanged": True, "id": original_block}
        else:
            cleanup_data = agent_http_request(
                "/block/break",
                method="POST",
                port=port,
                token=token,
                body={"x": pos[0], "y": pos[1], "z": pos[2], "drop": False},
            )
        add_smoke_step(report, "agent_cleanup_block", http_step_status(cleanup_data), cleanup_data, required=False)

    return finalize_interaction_report(report)


def finalize_interaction_report(report: dict[str, Any]) -> dict[str, Any]:
    finalize_smoke_report(report)
    if report.get("status") == "warning":
        report["ok"] = True
    return report


def add_smoke_step(
    report: dict[str, Any],
    name: str,
    status: str,
    data: dict[str, Any] | None = None,
    required: bool = True,
    error: str = "",
) -> None:
    step: dict[str, Any] = {"name": name, "status": status, "required": required}
    if error:
        step["error"] = error
    if data is not None:
        step["data"] = data
    report["steps"].append(step)


def count_result_issues(data: dict[str, Any]) -> int:
    summary = data.get("summary")
    if isinstance(summary, dict) and isinstance(summary.get("issues"), int):
        return int(summary["issues"])
    issues = data.get("issues")
    return len(issues) if isinstance(issues, list) else 0


def count_result_warnings(data: dict[str, Any]) -> int:
    summary = data.get("summary")
    if isinstance(summary, dict) and isinstance(summary.get("warnings"), int):
        return int(summary["warnings"])
    warnings = data.get("warnings")
    return len(warnings) if isinstance(warnings, list) else 0


def diagnostic_step_status(data: dict[str, Any]) -> str:
    status = data.get("status")
    if status in ("error", "failed", "crashed", "timeout") or count_result_issues(data):
        return "error"
    if status == "warning" or count_result_warnings(data):
        return "warning"
    return "ok"


def http_step_status(data: dict[str, Any]) -> str:
    if data.get("ok") is False:
        return "error"
    http_status = data.get("httpStatus")
    if isinstance(http_status, int) and http_status >= 400:
        return "error"
    return "ok"


def render_visual_check(data: dict[str, Any]) -> dict[str, Any] | None:
    render = data.get("render")
    if not isinstance(render, dict):
        return None
    visual_check = render.get("visualCheck")
    return visual_check if isinstance(visual_check, dict) else None


def held_item_render_status(data: dict[str, Any]) -> str:
    status = http_step_status(data)
    if status == "error":
        return status
    stack = data.get("stack")
    if isinstance(stack, dict) and stack.get("empty"):
        return "warning"
    render = data.get("render")
    model = render.get("model") if isinstance(render, dict) else None
    if isinstance(model, dict) and model.get("missingModel") is True:
        return "error"
    visual_check = render_visual_check(data)
    if isinstance(visual_check, dict) and visual_check.get("visibleLikely") is False:
        return "error"
    return "ok"


def block_render_status(data: dict[str, Any], expected_block: str | None = None) -> str:
    status = http_step_status(data)
    if status == "error":
        return status
    if expected_block and data.get("id") != expected_block:
        return "error"
    render = data.get("render")
    if isinstance(render, dict) and render.get("missingModel") is True:
        return "error"
    visual_check = render_visual_check(data)
    if expected_block and isinstance(visual_check, dict) and visual_check.get("visibleLikely") is False:
        return "error"
    return "ok"


def screenshot_status(data: dict[str, Any]) -> str:
    status = http_step_status(data)
    if status == "error":
        return status
    if data.get("blankLikely") is True:
        return "error"
    return "ok"


def recipe_status(data: dict[str, Any]) -> str:
    status = http_step_status(data)
    if status == "error":
        return status
    if data.get("matched") is not True or data.get("craftable") is not True:
        return "error"
    if data.get("expectedOutputMatches") is False:
        return "error"
    if data.get("recipeIdMatches") is False:
        return "error"
    if data.get("availableFromInventory") is False and data.get("missingItems"):
        return "error"
    return "ok"


def craft_status(data: dict[str, Any]) -> str:
    status = http_step_status(data)
    if status == "error":
        return status
    if data.get("matched") is not True or data.get("crafted") is not True:
        return "error"
    if data.get("expectedOutputMatches") is False:
        return "error"
    if data.get("availableFromInventory") is False and data.get("missingItems"):
        return "error"
    return "ok"


def interaction_check_status(data: dict[str, Any]) -> str:
    if data.get("ok") is False or data.get("status") == "error":
        return "error"
    if data.get("status") == "warning":
        return "warning"
    return "ok"


def use_legacy_recipe_default(profile: dict[str, Any] | None = None, capabilities_data: dict[str, Any] | None = None) -> bool:
    if profile and str(profile.get("minecraftVersion", "")).startswith("1.7."):
        return True
    if capabilities_data and str(capabilities_data.get("minecraftVersion", "")).startswith("1.7."):
        return True
    return False


def wait_agent_loaded(
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
    timeout_seconds: int = 120,
    poll_interval_seconds: float = 2.0,
) -> dict[str, Any]:
    started_at = time.time()
    deadline = started_at + max(1, int(timeout_seconds))
    poll_interval_seconds = max(0.25, float(poll_interval_seconds))
    last_health: dict[str, Any] | None = None
    last_state: dict[str, Any] | None = None

    while True:
        health = agent_http_request("/health", port=port, token=token)
        last_health = health
        if http_step_status(health) == "ok":
            state = agent_http_request("/state", port=port, token=token)
            last_state = state
            if http_step_status(state) == "ok" and state.get("loaded") is True:
                return {
                    "ok": True,
                    "status": "loaded",
                    "elapsedSeconds": round(time.time() - started_at, 3),
                    "health": health,
                    "state": state,
                }
        if time.time() >= deadline:
            return {
                "ok": False,
                "status": "timeout",
                "elapsedSeconds": round(time.time() - started_at, 3),
                "health": last_health,
                "state": last_state,
                "error": "agent did not report a loaded world before timeout",
            }
        time.sleep(poll_interval_seconds)


def wait_agent_available(
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
    timeout_seconds: int = 60,
    poll_interval_seconds: float = 2.0,
) -> dict[str, Any]:
    started_at = time.time()
    deadline = started_at + max(1, int(timeout_seconds))
    poll_interval_seconds = max(0.25, float(poll_interval_seconds))
    last_health: dict[str, Any] | None = None
    while True:
        health = agent_http_request("/health", port=port, token=token)
        last_health = health
        if http_step_status(health) == "ok":
            return {
                "ok": True,
                "status": "available",
                "elapsedSeconds": round(time.time() - started_at, 3),
                "health": health,
            }
        if time.time() >= deadline:
            return {
                "ok": False,
                "status": "timeout",
                "elapsedSeconds": round(time.time() - started_at, 3),
                "health": last_health,
                "error": "agent did not become reachable before timeout",
            }
        time.sleep(poll_interval_seconds)


def default_smoke_block_pos(state: dict[str, Any]) -> tuple[int, int, int]:
    player = state.get("player")
    block_pos = player.get("blockPos") if isinstance(player, dict) else None
    if not isinstance(block_pos, dict):
        raise ValueError("agent state does not include player.blockPos")
    return int(block_pos["x"]) + 1, int(block_pos["y"]), int(block_pos["z"])


def agent_block_probe(port: int, token: str | None, x: int, y: int, z: int) -> dict[str, Any]:
    return agent_http_request(
        "/world/snapshot",
        port=port,
        token=token,
        query={"radius": 0, "includeAir": "true", "x": x, "y": y, "z": z},
    )


def block_probe_id(data: dict[str, Any]) -> str | None:
    if data.get("id"):
        return str(data.get("id"))
    blocks = data.get("blocks")
    if isinstance(blocks, list) and blocks and isinstance(blocks[0], dict):
        block_id = blocks[0].get("id")
        return str(block_id) if block_id else None
    return None


def finalize_smoke_report(report: dict[str, Any]) -> dict[str, Any]:
    required_errors = [step for step in report["steps"] if step.get("required") and step.get("status") == "error"]
    warnings = [step for step in report["steps"] if step.get("status") == "warning"]
    optional_errors = [step for step in report["steps"] if not step.get("required") and step.get("status") == "error"]
    report["ok"] = not required_errors
    report["status"] = "error" if required_errors else "warning" if warnings or optional_errors else "ok"
    report["summary"] = {
        "steps": len(report["steps"]),
        "passed": sum(1 for step in report["steps"] if step.get("status") == "ok"),
        "warnings": len(warnings) + len(optional_errors),
        "failed": len(required_errors),
        "skipped": sum(1 for step in report["steps"] if step.get("status") == "skipped"),
    }
    return report


def agent_smoke_test(
    ctx: DiagnosticContext,
    profile_id: str | None = None,
    jar_path: str | None = None,
    launch: bool = False,
    quick_play_singleplayer: str | None = None,
    world_name: str | None = None,
    create_world: bool = False,
    world_gamemode: str = "creative",
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
    timeout_seconds: int = 120,
    poll_interval_seconds: float = 2.0,
    hotbar_slot: int | None = 0,
    give_item: str | None = None,
    give_count: int = 1,
    give_slot: int | None = 0,
    give_select: bool = True,
    give_replace: bool = True,
    release_input: bool = True,
    check_held_item: bool = True,
    take_screenshot: bool = True,
    recipe_items: list[Any] | None = None,
    recipe_width: int = 1,
    recipe_height: int = 1,
    expected_output: str | None = None,
    recipe_id: str | None = None,
    require_inventory: bool = False,
    craft_recipe: bool = False,
    craft_give_inputs: bool = True,
    crafts: int = 1,
    block: str = "minecraft:stone",
    block_x: int | None = None,
    block_y: int | None = None,
    block_z: int | None = None,
    check_use_item: bool = True,
    check_use_block: bool = True,
    cleanup: bool = True,
    include_logs: bool = True,
) -> dict[str, Any]:
    using_default_recipe = recipe_items is None
    if recipe_items is None:
        recipe_items = ["minecraft:oak_log"]
    if expected_output is None and using_default_recipe:
        expected_output = "minecraft:oak_planks"

    report: dict[str, Any] = {
        "ok": False,
        "status": "error",
        "config": {
            "jarPath": jar_path or "",
            "launch": launch,
            "quickPlaySingleplayer": quick_play_singleplayer or "",
            "worldName": world_name or "",
            "createWorld": create_world,
            "worldGamemode": world_gamemode,
            "port": port,
            "timeoutSeconds": timeout_seconds,
            "hotbarSlot": hotbar_slot,
            "giveItem": give_item or "",
            "giveCount": give_count,
            "giveSlot": give_slot,
            "giveSelect": give_select,
            "giveReplace": give_replace,
            "releaseInput": release_input,
            "recipeItems": recipe_items,
            "recipeWidth": recipe_width,
            "recipeHeight": recipe_height,
            "expectedOutput": expected_output or "",
            "recipeId": recipe_id or "",
            "requireInventory": require_inventory,
            "craftRecipe": craft_recipe,
            "craftGiveInputs": craft_give_inputs,
            "crafts": crafts,
            "block": block,
            "checkUseItem": check_use_item,
            "checkUseBlock": check_use_block,
            "cleanup": cleanup,
            "includeLogs": include_logs,
        },
        "steps": [],
    }

    profile: dict[str, Any] | None = None
    try:
        profile = find_profile(ctx, profile_id)
        report["profile"] = profile_brief(profile)
        if using_default_recipe and use_legacy_recipe_default(profile):
            recipe_items = ["minecraft:log"]
            expected_output = "minecraft:planks"
            report["config"]["recipeItems"] = recipe_items
            report["config"]["expectedOutput"] = expected_output
    except Exception as exc:
        if profile_id or launch:
            add_smoke_step(report, "resolve_profile", "error", required=True, error=str(exc))
            return finalize_smoke_report(report)
        add_smoke_step(report, "resolve_profile", "warning", required=False, error=str(exc))

    if jar_path:
        try:
            mod_data = inspect_mod(jar_path, profile)
            add_smoke_step(report, "diagnose_mod", diagnostic_step_status(mod_data), mod_data)
        except Exception as exc:
            add_smoke_step(report, "diagnose_mod", "error", error=str(exc))
        try:
            content_data = diagnose_mod_content(jar_path)
            add_smoke_step(report, "diagnose_mod_content", diagnostic_step_status(content_data), content_data)
        except Exception as exc:
            add_smoke_step(report, "diagnose_mod_content", "error", error=str(exc))

    if profile is not None:
        try:
            profile_data = diagnose_profile(ctx, profile.get("id"), include_logs=False)
            add_smoke_step(report, "diagnose_profile_static", diagnostic_step_status(profile_data), profile_data)
        except Exception as exc:
            add_smoke_step(report, "diagnose_profile_static", "error", error=str(exc))

    if launch:
        try:
            launch_data = launch_profile(ctx, profile["id"] if profile else profile_id, quick_play_singleplayer)
            add_smoke_step(report, "launch_profile", "ok" if launch_data.get("ok") is not False and not launch_data.get("error") else "error", launch_data)
            if report["steps"][-1]["status"] == "error":
                return finalize_smoke_report(report)
        except Exception as exc:
            add_smoke_step(report, "launch_profile", "error", error=str(exc))
            return finalize_smoke_report(report)

        try:
            ready_data = wait_profile_ready(ctx, profile["id"] if profile else profile_id, timeout_seconds, poll_interval_seconds)
            ready_status = "ok" if ready_data.get("status") == "ready" else diagnostic_step_status(ready_data)
            add_smoke_step(report, "wait_profile_ready", ready_status, ready_data)
            if ready_status == "error" and ready_data.get("status") in ("failed", "crashed"):
                return finalize_smoke_report(report)
        except Exception as exc:
            add_smoke_step(report, "wait_profile_ready", "warning", required=False, error=str(exc))

    if world_name:
        available_data = wait_agent_available(port, token, min(timeout_seconds, 60), poll_interval_seconds)
        add_smoke_step(report, "wait_agent_available", "ok" if available_data.get("ok") else "error", available_data)
        if not available_data.get("ok"):
            return finalize_smoke_report(report)
        open_capabilities = agent_capabilities(port=port, token=token)
        add_smoke_step(report, "agent_capabilities_before_world_open", http_step_status(open_capabilities), open_capabilities, required=False)
        if agent_supports(open_capabilities, "autoWorldOpen", default=False):
            open_data = agent_world_open(
                world_name,
                create_world,
                world_gamemode,
                None,
                world_name,
                port,
                token,
            )
            add_smoke_step(report, "agent_world_open", http_step_status(open_data), open_data)
            if http_step_status(open_data) == "error":
                return finalize_smoke_report(report)
        else:
            add_smoke_step(
                report,
                "agent_world_open",
                "error",
                data={"ok": False, "reason": "agent capability autoWorldOpen=false"},
            )
            return finalize_smoke_report(report)

    loaded_data = wait_agent_loaded(port, token, timeout_seconds, poll_interval_seconds)
    add_smoke_step(report, "wait_agent_loaded", "ok" if loaded_data.get("ok") else "error", loaded_data)
    if not loaded_data.get("ok"):
        return finalize_smoke_report(report)

    health_data = agent_http_request("/health", port=port, token=token)
    add_smoke_step(report, "agent_health", http_step_status(health_data), health_data)

    capabilities_data = agent_capabilities(port=port, token=token)
    add_smoke_step(report, "agent_capabilities", http_step_status(capabilities_data), capabilities_data)
    if using_default_recipe and use_legacy_recipe_default(profile, capabilities_data):
        recipe_items = ["minecraft:log"]
        expected_output = "minecraft:planks"
        report["config"]["recipeItems"] = recipe_items
        report["config"]["expectedOutput"] = expected_output

    if release_input:
        if agent_supports(capabilities_data, "inputRelease", default=False):
            release_data = agent_http_request("/input/release", method="POST", port=port, token=token)
            add_smoke_step(report, "agent_release_input", http_step_status(release_data), release_data, required=False)
        else:
            add_smoke_step(report, "agent_release_input", "skipped", required=False, data={"reason": "agent capability inputRelease=false"})
    else:
        add_smoke_step(report, "agent_release_input", "skipped", required=False)

    state_data = agent_http_request("/state", port=port, token=token)
    add_smoke_step(report, "agent_state", http_step_status(state_data), state_data)
    if http_step_status(state_data) == "error" or state_data.get("loaded") is not True:
        return finalize_smoke_report(report)

    if agent_supports(capabilities_data, "waitTicks", default=False):
        wait_data = agent_wait_ticks(2, 10, port, token)
        wait_status = "ok" if wait_data.get("ok") is not False and wait_data.get("complete") is not False else "error"
        add_smoke_step(report, "agent_wait_ticks", wait_status, wait_data)
    else:
        add_smoke_step(report, "agent_wait_ticks", "skipped", required=False, data={"reason": "agent capability waitTicks=false"})

    inventory_data = agent_http_request("/inventory", port=port, token=token)
    add_smoke_step(report, "agent_inventory", http_step_status(inventory_data), inventory_data)

    if give_item:
        if agent_supports(capabilities_data, "giveItem"):
            target_slot = give_slot if give_slot is not None else (hotbar_slot if hotbar_slot is not None else 0)
            give_data = agent_http_request(
                "/inventory/give",
                method="POST",
                port=port,
                token=token,
                body={
                    "item": give_item,
                    "count": int(give_count),
                    "slot": int(target_slot),
                    "select": bool(give_select),
                    "replace": bool(give_replace),
                },
            )
            add_smoke_step(report, "agent_give_item", http_step_status(give_data), give_data)
            if http_step_status(give_data) == "error":
                return finalize_smoke_report(report)
            if give_select:
                hotbar_slot = None
        else:
            add_smoke_step(report, "agent_give_item", "skipped", required=False, data={"reason": "agent capability giveItem=false"})

    if check_held_item:
        if hotbar_slot is not None and agent_supports(capabilities_data, "selectHotbar"):
            select_data = agent_http_request("/hotbar/select", method="POST", port=port, token=token, body={"slot": int(hotbar_slot)})
            add_smoke_step(report, "agent_select_hotbar_slot", http_step_status(select_data), select_data)
        if agent_supports(capabilities_data, "heldItemRender"):
            held_data = agent_http_request("/render/held-item", port=port, token=token, query={"hand": "main"})
            add_smoke_step(report, "agent_held_item_render", held_item_render_status(held_data), held_data, required=False)
        else:
            add_smoke_step(report, "agent_held_item_render", "skipped", required=False, data={"reason": "agent capability heldItemRender=false"})
    else:
        add_smoke_step(report, "agent_held_item_render", "skipped", required=False)

    if take_screenshot:
        if agent_supports(capabilities_data, "screenshot"):
            screenshot_data = agent_http_request("/render/screenshot", port=port, token=token)
            add_smoke_step(report, "agent_screenshot", screenshot_status(screenshot_data), screenshot_data)
        else:
            add_smoke_step(report, "agent_screenshot", "skipped", required=False, data={"reason": "agent capability screenshot=false"})
    else:
        add_smoke_step(report, "agent_screenshot", "skipped", required=False)

    if recipe_items:
        if agent_supports(capabilities_data, "recipeCheck"):
            recipe_data = agent_http_request(
                "/recipe/check",
                method="POST",
                port=port,
                token=token,
                body={
                    "width": int(recipe_width),
                    "height": int(recipe_height),
                    "items": recipe_items,
                    "expectedOutput": expected_output or "",
                    "recipeId": recipe_id or "",
                    "requireInventory": bool(require_inventory),
                },
            )
            add_smoke_step(report, "agent_recipe_check", recipe_status(recipe_data), recipe_data)
        else:
            add_smoke_step(report, "agent_recipe_check", "skipped", required=False, data={"reason": "agent capability recipeCheck=false"})
    else:
        add_smoke_step(report, "agent_recipe_check", "skipped", required=False)

    if recipe_items and craft_recipe:
        if agent_supports(capabilities_data, "craftRecipe", default=False):
            if craft_give_inputs:
                if agent_supports(capabilities_data, "giveItem", default=False):
                    for index, spec in enumerate(recipe_give_specs(recipe_items, crafts)):
                        give_body = {
                            "item": spec["item"],
                            "count": int(spec["count"]),
                            "slot": index,
                            "select": False,
                            "replace": True,
                        }
                        if int(spec.get("damage", 0)):
                            give_body["damage"] = int(spec["damage"])
                            give_body["metadata"] = int(spec["damage"])
                        give_data = agent_http_request(
                            "/inventory/give",
                            method="POST",
                            port=port,
                            token=token,
                            body=give_body,
                        )
                        add_smoke_step(report, "agent_craft_give_input", http_step_status(give_data), give_data)
                        if http_step_status(give_data) == "error":
                            return finalize_smoke_report(report)
                else:
                    add_smoke_step(report, "agent_craft_give_input", "error", data={"reason": "agent capability giveItem=false"})
                    return finalize_smoke_report(report)
            craft_data = agent_craft_recipe(
                recipe_items,
                recipe_width,
                recipe_height,
                expected_output,
                crafts,
                True,
                True,
                True,
                None,
                False,
                port,
                token,
            )
            add_smoke_step(report, "agent_craft_recipe", craft_status(craft_data), craft_data)
        else:
            add_smoke_step(report, "agent_craft_recipe", "skipped", required=False, data={"reason": "agent capability craftRecipe=false"})
    elif craft_recipe:
        add_smoke_step(report, "agent_craft_recipe", "skipped", required=False, data={"reason": "recipe_items is empty"})
    else:
        add_smoke_step(report, "agent_craft_recipe", "skipped", required=False)

    try:
        if block_x is None or block_y is None or block_z is None:
            block_x, block_y, block_z = default_smoke_block_pos(state_data)
        report["config"]["blockPos"] = {"x": block_x, "y": block_y, "z": block_z}
        if agent_supports(capabilities_data, "blockRender"):
            before_data = agent_http_request(
                "/render/block",
                port=port,
                token=token,
                query={"x": block_x, "y": block_y, "z": block_z},
            )
            add_smoke_step(report, "agent_block_render_before", block_render_status(before_data), before_data, required=False)
            original_block = before_data.get("id") if before_data.get("ok") is not False else None
        elif agent_supports(capabilities_data, "worldSnapshot"):
            before_data = agent_block_probe(port, token, int(block_x), int(block_y), int(block_z))
            add_smoke_step(report, "agent_block_snapshot_before", http_step_status(before_data), before_data, required=False)
            original_block = block_probe_id(before_data) if before_data.get("ok") is not False else None
        else:
            before_data = {"ok": True, "skipped": True, "reason": "agent has no blockRender or worldSnapshot capability"}
            add_smoke_step(report, "agent_block_probe_before", "skipped", before_data, required=False)
            original_block = None

        if agent_supports(capabilities_data, "placeBlock"):
            place_data = agent_http_request(
                "/block/place",
                method="POST",
                port=port,
                token=token,
                body={"x": block_x, "y": block_y, "z": block_z, "block": block},
            )
            place_status = http_step_status(place_data)
            if place_status == "ok" and place_data.get("id") != block:
                place_status = "error"
            add_smoke_step(report, "agent_place_block", place_status, place_data)
        else:
            add_smoke_step(report, "agent_place_block", "skipped", required=False, data={"reason": "agent capability placeBlock=false"})
            return finalize_smoke_report(report)

        if agent_supports(capabilities_data, "blockRender"):
            render_data = agent_http_request(
                "/render/block",
                port=port,
                token=token,
                query={"x": block_x, "y": block_y, "z": block_z},
            )
            add_smoke_step(report, "agent_block_render", block_render_status(render_data, block), render_data)
        elif agent_supports(capabilities_data, "worldSnapshot"):
            render_data = agent_block_probe(port, token, int(block_x), int(block_y), int(block_z))
            snapshot_status = http_step_status(render_data)
            if snapshot_status == "ok" and block_probe_id(render_data) != block:
                snapshot_status = "error"
            add_smoke_step(report, "agent_block_snapshot", snapshot_status, render_data)
        else:
            add_smoke_step(report, "agent_block_render", "skipped", required=False, data={"reason": "agent has no blockRender or worldSnapshot capability"})

        if check_use_block:
            if agent_supports(capabilities_data, "useBlock", default=False):
                use_block_data = agent_interaction_check(
                    "block",
                    x=int(block_x),
                    y=int(block_y),
                    z=int(block_z),
                    hand="main",
                    side="up",
                    slot=hotbar_slot,
                    wait_ticks=2,
                    timeout_seconds=10,
                    require_effect=False,
                    port=port,
                    token=token,
                )
                add_smoke_step(report, "agent_use_block_check", interaction_check_status(use_block_data), use_block_data, required=False)
            else:
                add_smoke_step(report, "agent_use_block_check", "skipped", required=False, data={"reason": "agent capability useBlock=false"})
        else:
            add_smoke_step(report, "agent_use_block_check", "skipped", required=False)

        if cleanup:
            if original_block and original_block != "minecraft:air" and original_block != block:
                cleanup_data = agent_http_request(
                    "/block/place",
                    method="POST",
                    port=port,
                    token=token,
                    body={"x": block_x, "y": block_y, "z": block_z, "block": original_block},
                )
                cleanup_status = http_step_status(cleanup_data)
            elif original_block == block:
                cleanup_data = {"ok": True, "unchanged": True, "id": original_block}
                cleanup_status = "ok"
            else:
                cleanup_data = agent_http_request(
                    "/block/break",
                    method="POST",
                    port=port,
                    token=token,
                    body={"x": block_x, "y": block_y, "z": block_z, "drop": False},
                )
                cleanup_status = http_step_status(cleanup_data)
            add_smoke_step(report, "agent_cleanup_block", cleanup_status, cleanup_data, required=False)
    except Exception as exc:
        add_smoke_step(report, "agent_block_flow", "error", error=str(exc))

    if check_use_item and give_item:
        if agent_supports(capabilities_data, "useItem", default=False):
            target_slot = give_slot if give_slot is not None else (hotbar_slot if hotbar_slot is not None else 0)
            use_item_data = agent_interaction_check(
                "item",
                slot=int(target_slot),
                hand="main",
                wait_ticks=2,
                timeout_seconds=10,
                require_effect=False,
                port=port,
                token=token,
            )
            add_smoke_step(report, "agent_use_item_check", interaction_check_status(use_item_data), use_item_data, required=False)
        else:
            add_smoke_step(report, "agent_use_item_check", "skipped", required=False, data={"reason": "agent capability useItem=false"})
    else:
        add_smoke_step(report, "agent_use_item_check", "skipped", required=False)

    if include_logs and profile is not None:
        try:
            logs_data = diagnose_profile(ctx, profile.get("id"), include_logs=True, log_scope="latest_run")
            add_smoke_step(report, "diagnose_profile_latest_run", diagnostic_step_status(logs_data), logs_data, required=False)
        except Exception as exc:
            add_smoke_step(report, "diagnose_profile_latest_run", "warning", required=False, error=str(exc))

    return finalize_smoke_report(report)


def mod_dev_loop(
    ctx: DiagnosticContext,
    project_dir: str,
    profile_id: str | None = None,
    build: bool = True,
    build_command: str | None = None,
    artifact_glob: str | None = None,
    jar_path: str | None = None,
    import_mod: bool = True,
    import_file_name: str | None = None,
    replace_mod: bool = True,
    install_agent: bool = True,
    launch: bool = True,
    keep_running: bool = False,
    quick_play_singleplayer: str | None = None,
    world_name: str | None = "Power Mine Codex Dev Test",
    create_world: bool = True,
    world_gamemode: str = "creative",
    port: int = DEFAULT_AGENT_PORT,
    token: str | None = None,
    timeout_seconds: int = 120,
    poll_interval_seconds: float = 2.0,
    use_sample_targets: bool = True,
    give_item: str | None = None,
    give_count: int = 1,
    give_slot: int | None = 0,
    recipe_items: list[Any] | None = None,
    recipe_width: int = 1,
    recipe_height: int = 1,
    expected_output: str | None = None,
    block: str | None = None,
    cleanup: bool = True,
    include_logs: bool = True,
    check_held_item: bool = True,
    take_screenshot: bool = True,
    check_use_item: bool = True,
    check_use_block: bool = True,
    craft_recipe: bool = True,
    craft_give_inputs: bool = True,
    crafts: int = 1,
) -> dict[str, Any]:
    project = Path(project_dir).expanduser().resolve()
    report: dict[str, Any] = {
        "ok": False,
        "status": "error",
        "config": {
            "projectDir": str(project),
            "profileId": profile_id or "",
            "build": build,
            "buildCommand": build_command or "",
            "artifactGlob": artifact_glob or "",
            "jarPath": jar_path or "",
            "importMod": import_mod,
            "importFileName": import_file_name or "",
            "replaceMod": replace_mod,
            "installAgent": install_agent,
            "launch": launch,
            "keepRunning": keep_running,
            "quickPlaySingleplayer": quick_play_singleplayer or "",
            "worldName": world_name or "",
            "createWorld": create_world,
            "worldGamemode": world_gamemode,
            "port": port,
            "timeoutSeconds": timeout_seconds,
            "useSampleTargets": use_sample_targets,
            "giveItem": give_item or "",
            "giveCount": give_count,
            "giveSlot": give_slot,
            "recipeItems": recipe_items,
            "recipeWidth": recipe_width,
            "recipeHeight": recipe_height,
            "expectedOutput": expected_output or "",
            "block": block or "",
            "cleanup": cleanup,
            "includeLogs": include_logs,
            "checkHeldItem": check_held_item,
            "takeScreenshot": take_screenshot,
            "checkUseItem": check_use_item,
            "checkUseBlock": check_use_block,
            "craftRecipe": craft_recipe,
            "craftGiveInputs": craft_give_inputs,
            "crafts": crafts,
        },
        "steps": [],
    }

    try:
        profile = find_profile(ctx, profile_id)
        report["profile"] = profile_brief(profile)
    except Exception as exc:
        add_smoke_step(report, "resolve_profile", "error", error=str(exc))
        return finalize_smoke_report(report)

    resolved_jar = Path(jar_path).expanduser().resolve() if jar_path else None
    if build:
        try:
            build_data = build_mod_project(str(project), build_command, artifact_glob, max(timeout_seconds, 60))
            add_smoke_step(report, "build_project", "ok" if build_data.get("ok") else "error", build_data)
            if not build_data.get("ok"):
                return finalize_smoke_report(report)
            resolved_jar = Path(str(build_data["jarPath"])).expanduser().resolve()
        except Exception as exc:
            add_smoke_step(report, "build_project", "error", error=str(exc))
            return finalize_smoke_report(report)
    elif resolved_jar is None:
        try:
            artifact_data = find_built_mod_project_jar(str(project), artifact_glob)
            add_smoke_step(report, "find_built_artifact", "ok" if artifact_data.get("ok") else "error", artifact_data)
            if not artifact_data.get("ok"):
                return finalize_smoke_report(report)
            resolved_jar = Path(str(artifact_data["jarPath"])).expanduser().resolve()
        except Exception as exc:
            add_smoke_step(report, "find_built_artifact", "error", error=str(exc))
            return finalize_smoke_report(report)

    if resolved_jar is None:
        add_smoke_step(report, "resolve_jar", "error", error="no jar_path or built artifact resolved")
        return finalize_smoke_report(report)
    report["config"]["jarPath"] = str(resolved_jar)

    try:
        mod_data = inspect_mod(str(resolved_jar), profile)
        add_smoke_step(report, "diagnose_mod", diagnostic_step_status(mod_data), mod_data)
        if diagnostic_step_status(mod_data) == "error":
            return finalize_smoke_report(report)
    except Exception as exc:
        add_smoke_step(report, "diagnose_mod", "error", error=str(exc))
        return finalize_smoke_report(report)

    try:
        content_data = diagnose_mod_content(str(resolved_jar))
        add_smoke_step(report, "diagnose_mod_content", diagnostic_step_status(content_data), content_data, required=False)
    except Exception as exc:
        add_smoke_step(report, "diagnose_mod_content", "warning", required=False, error=str(exc))

    inferred_mod_id = first_declared_mod_id(mod_data)
    if inferred_mod_id:
        report["modId"] = inferred_mod_id
    if use_sample_targets and inferred_mod_id:
        if give_item is None:
            give_item = f"{inferred_mod_id}:sample_item"
        if recipe_items is None:
            recipe_items = ["minecraft:emerald"]
        if expected_output is None:
            expected_output = give_item
        if block is None:
            block = f"{inferred_mod_id}:sample_block"
    report["config"]["giveItem"] = give_item or ""
    report["config"]["recipeItems"] = recipe_items
    report["config"]["expectedOutput"] = expected_output or ""
    report["config"]["block"] = block or "minecraft:stone"

    if import_mod:
        try:
            import_data = import_profile_mod(ctx, profile["id"], str(resolved_jar), import_file_name, True, replace_mod)
            add_smoke_step(report, "import_profile_mod", diagnostic_step_status(import_data), import_data)
            if diagnostic_step_status(import_data) == "error":
                return finalize_smoke_report(report)
        except Exception as exc:
            add_smoke_step(report, "import_profile_mod", "error", error=str(exc))
            return finalize_smoke_report(report)

    if install_agent:
        try:
            agent_data = install_agent_mod(ctx, profile["id"], True, True, None)
            add_smoke_step(report, "install_agent_mod", diagnostic_step_status(agent_data), agent_data)
            if diagnostic_step_status(agent_data) == "error" or agent_data.get("ok") is False:
                return finalize_smoke_report(report)
        except Exception as exc:
            add_smoke_step(report, "install_agent_mod", "error", error=str(exc))
            return finalize_smoke_report(report)

    launched_pid: int | None = None
    try:
        smoke_data = agent_smoke_test(
            ctx,
            profile["id"],
            str(resolved_jar),
            launch,
            quick_play_singleplayer,
            world_name,
            create_world,
            world_gamemode,
            port,
            token,
            timeout_seconds,
            poll_interval_seconds,
            0,
            give_item or None,
            give_count,
            give_slot,
            True,
            True,
            True,
            check_held_item,
            take_screenshot,
            recipe_items,
            recipe_width,
            recipe_height,
            expected_output,
            None,
            False,
            craft_recipe,
            craft_give_inputs,
            crafts,
            block or "minecraft:stone",
            None,
            None,
            None,
            check_use_item,
            check_use_block,
            cleanup,
            include_logs,
        )
        launched_pid = extract_launch_pid_from_smoke(smoke_data)
        add_smoke_step(report, "agent_smoke_test", smoke_data.get("status", "error"), smoke_data)
    finally:
        if launch and launched_pid and not keep_running:
            stop_data = terminate_process(launched_pid)
            add_smoke_step(report, "stop_launched_profile", "ok" if stop_data.get("ok") else "warning", stop_data, required=False)

    return finalize_smoke_report(report)


LOG_PATTERNS = [
    ("error", "unsupported_java", re.compile(r"UnsupportedClassVersionError|class file version", re.I), "Java version mismatch"),
    ("error", "missing_class", re.compile(r"NoClassDefFoundError|ClassNotFoundException", re.I), "Missing dependency or wrong loader/version"),
    ("error", "mixin_failed", re.compile(r"Mixin apply failed|MixinTransformerError|InjectionError|InvalidMixinException", re.I), "Mixin failure"),
    ("error", "missing_dependency", re.compile(r"requires.*(?:mod|dependency)|missing.*(?:mod|dependency)|ModResolutionException", re.I), "Missing dependency"),
    ("error", "duplicate_mod", re.compile(r"duplicate mod|DuplicateModsFoundException|Duplicate mod", re.I), "Duplicate mod"),
    ("error", "mod_loading", re.compile(r"ModLoadingException|Loading errors encountered|Failed to load mod", re.I), "Mod loading failure"),
    (
        "warning",
        "auth_warning",
        re.compile(
            r"Failed to verify authentication|authentication servers are down|invalid session|"
            r"MinecraftClientHttpException.*(?:HTTP_ERROR|Status:\s*401|status=401)|Status:\s*401",
            re.I,
        ),
        "Authentication warning",
    ),
    ("error", "exception", re.compile(r"\b(FATAL|ERROR)\b|Exception in thread|Caused by:", re.I), "Exception or error line"),
]

IGNORED_LOG_PATTERNS = [
    re.compile(r"Added class metadata for .*(?:ClassNotFoundException|NoClassDefFoundError) to metadata cache", re.I),
    re.compile(r"FML appears to be missing any signature data", re.I),
    re.compile(r"THIS IS NOT A ERROR", re.I),
    re.compile(r"A detailed walkthrough of the error, its code path and all known details is as follows:", re.I),
    re.compile(r"Attempting connection with missing mods \[(?:minecraft,\s*)?mcp,\s*FML,\s*forge(?:,\s*[^\]]+)?\] at (?:CLIENT|SERVER)", re.I),
    re.compile(r"Attempting connection with missing mods \[\] at (?:CLIENT|SERVER)", re.I),
    re.compile(r"Couldn't initialize twitch stream", re.I),
    re.compile(r"Could not save the splash\.properties file", re.I),
    re.compile(r"Forge Version Check", re.I),
    re.compile(r"Expected BEGIN_OBJECT but was STRING at line 1 column 1", re.I),
]

READY_LOG_PATTERNS = [
    ("resource_reload", re.compile(r"Reloading ResourceManager", re.I), "Resource manager reload started"),
    ("sound_engine", re.compile(r"Sound engine started|OpenAL initialized", re.I), "Client sound engine initialized"),
    ("texture_atlas", re.compile(r"Created:\s+.*textures/atlas", re.I), "Client texture atlas created"),
    ("integrated_server", re.compile(r"Started integrated server|Preparing spawn area", re.I), "Integrated world is loading"),
]

EARLY_FAILURE_LOG_KEYS = {"unsupported_java", "missing_class", "mixin_failed", "missing_dependency", "duplicate_mod", "mod_loading"}


def read_log_tail(path: Path, max_bytes: int) -> tuple[str, bool]:
    if path.suffix == ".gz":
        with gzip.open(path, "rb") as handle:
            raw = handle.read(max_bytes + 1)
        return raw[:max_bytes].decode("utf-8", errors="replace"), len(raw) > max_bytes
    size = path.stat().st_size
    with path.open("rb") as handle:
        if size > max_bytes:
            handle.seek(-max_bytes, os.SEEK_END)
        raw = handle.read(max_bytes + 1)
    if len(raw) > max_bytes:
        raw = raw[-max_bytes:]
    return raw.decode("utf-8", errors="replace"), size > max_bytes


def recent_log_paths(profile: dict[str, Any], limit: int = 5) -> list[Path]:
    game_dir = Path(str(profile.get("gameDir", ""))).expanduser()
    candidates: list[Path] = []
    for folder, suffixes in ((game_dir / "logs", (".log", ".log.gz")), (game_dir / "crash-reports", (".txt",))):
        if folder.is_dir():
            candidates.extend([path for path in folder.iterdir() if path.is_file() and path.name.lower().endswith(suffixes)])
    if game_dir.is_dir():
        candidates.extend([
            path
            for path in game_dir.iterdir()
            if path.is_file() and path.name.startswith("hs_err_pid") and path.name.lower().endswith(".log")
        ])
    return sorted(candidates, key=lambda path: path.stat().st_mtime, reverse=True)[:limit]


def latest_launch_log_path(profile: dict[str, Any]) -> Path | None:
    game_dir = Path(str(profile.get("gameDir", ""))).expanduser()
    logs_dir = game_dir / "logs"
    if not logs_dir.is_dir():
        return None
    candidates = [
        path
        for path in logs_dir.iterdir()
        if path.is_file() and path.name.startswith("power-mine-headless-") and path.name.lower().endswith(".log")
    ]
    if not candidates:
        return None
    return sorted(candidates, key=lambda path: path.stat().st_mtime, reverse=True)[0]


def latest_run_log_paths(profile: dict[str, Any], limit: int = 8) -> list[Path]:
    game_dir = Path(str(profile.get("gameDir", ""))).expanduser()
    launch_log = latest_launch_log_path(profile)
    if launch_log is None:
        latest = game_dir / "logs" / "latest.log"
        if latest.is_file():
            return [latest]
        return recent_log_paths(profile, limit)

    cutoff = launch_log.stat().st_mtime - LATEST_RUN_LOG_SLOP_SECONDS
    candidates: list[Path] = [launch_log]
    for folder, suffixes in ((game_dir / "logs", (".log", ".log.gz")), (game_dir / "crash-reports", (".txt",))):
        if not folder.is_dir():
            continue
        candidates.extend(
            path
            for path in folder.iterdir()
            if path.is_file() and path.name.lower().endswith(suffixes) and path.stat().st_mtime >= cutoff
        )
    if game_dir.is_dir():
        candidates.extend(
            path
            for path in game_dir.iterdir()
            if path.is_file()
            and path.name.startswith("hs_err_pid")
            and path.name.lower().endswith(".log")
            and path.stat().st_mtime >= cutoff
        )

    unique = sorted(set(candidates), key=lambda path: path.stat().st_mtime, reverse=True)
    return unique[:limit]


def profile_log_paths(profile: dict[str, Any], log_scope: str, limit: int = 8) -> list[Path]:
    if log_scope == "recent":
        return recent_log_paths(profile, limit)
    if log_scope == "latest_run":
        return latest_run_log_paths(profile, limit)
    raise ValueError("log_scope must be 'recent' or 'latest_run'")


def analyze_log(path: Path, max_bytes: int) -> dict[str, Any]:
    content, truncated = read_log_tail(path, max_bytes)
    matches = []
    for index, line in enumerate(content.splitlines(), start=1):
        if "<log4j:Event " in line and "level=\"ERROR\"" in line:
            continue
        if any(pattern.search(line) for pattern in IGNORED_LOG_PATTERNS):
            continue
        for severity, key, pattern, hint in LOG_PATTERNS:
            if pattern.search(line):
                matches.append({
                    "severity": severity,
                    "key": key,
                    "hint": hint,
                    "line": index,
                    "text": line.strip()[:500],
                })
                break
    return {
        "path": str(path),
        "fileName": path.name,
        "size": path.stat().st_size,
        "truncated": truncated,
        "matches": matches[:80],
    }


def analyze_ready_signals(path: Path, max_bytes: int) -> list[dict[str, Any]]:
    content, _ = read_log_tail(path, max_bytes)
    matches = []
    for index, line in enumerate(content.splitlines(), start=1):
        for key, pattern, hint in READY_LOG_PATTERNS:
            if pattern.search(line):
                matches.append({"key": key, "hint": hint, "line": index, "text": line.strip()[:500]})
                break
    return matches[:40]


def compact_ready_signals(signals: list[dict[str, Any]], limit: int = 12) -> list[dict[str, Any]]:
    compacted = []
    seen: set[tuple[str, str]] = set()
    for signal in signals:
        key = (str(signal.get("key", "")), str(signal.get("fileName", "")))
        if key in seen:
            continue
        seen.add(key)
        compacted.append(signal)
        if len(compacted) >= limit:
            break
    return compacted


def is_crash_artifact(path: Path) -> bool:
    return path.parent.name == "crash-reports" or (path.name.startswith("hs_err_pid") and path.name.lower().endswith(".log"))


def diagnose_profile(
    ctx: DiagnosticContext,
    profile_id: str | None = None,
    include_logs: bool = True,
    max_log_bytes: int = MAX_LOG_BYTES,
    log_scope: str = "recent",
) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    mods = [inspect_mod(str(path), profile) for path in list_profile_mod_paths(profile)]
    add_profile_dependency_diagnostics(profile, mods)
    logs = [analyze_log(path, max_log_bytes) for path in profile_log_paths(profile, log_scope) if include_logs] if include_logs else []
    log_error_count = sum(
        1
        for log in logs
        for match in log.get("matches", [])
        if match.get("severity") == "error"
    )
    log_warning_count = sum(
        1
        for log in logs
        for match in log.get("matches", [])
        if match.get("severity") == "warning"
    )
    issue_count = sum(len(mod.get("issues", [])) for mod in mods) + log_error_count
    warning_count = sum(len(mod.get("warnings", [])) for mod in mods) + log_warning_count
    status = "error" if issue_count else "warning" if warning_count else "ok"
    return {
        "status": status,
        "profile": profile_brief(profile),
        "summary": {
            "mods": len(mods),
            "issues": issue_count,
            "warnings": warning_count,
            "logsAnalyzed": len(logs),
            "logScope": log_scope,
            "logErrors": log_error_count,
            "logWarnings": log_warning_count,
        },
        "mods": mods,
        "logs": logs,
    }


def wait_profile_ready(
    ctx: DiagnosticContext,
    profile_id: str | None = None,
    timeout_seconds: int = 90,
    poll_interval_seconds: float = 2.0,
    max_log_bytes: int = MAX_LOG_BYTES,
) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    started_at = time.time()
    deadline = started_at + max(1, int(timeout_seconds))
    poll_interval_seconds = max(0.25, float(poll_interval_seconds))
    last_logs: list[dict[str, Any]] = []
    last_ready_signals: list[dict[str, Any]] = []

    while True:
        paths = latest_run_log_paths(profile)
        logs = [analyze_log(path, max_log_bytes) for path in paths]
        ready_signals = []
        for path in paths:
            for signal in analyze_ready_signals(path, max_log_bytes):
                item = dict(signal)
                item["fileName"] = path.name
                item["path"] = str(path)
                ready_signals.append(item)
        ready_signals = compact_ready_signals(ready_signals)

        crash_artifacts = [path for path in paths if is_crash_artifact(path)]
        early_failures = [
            match
            for log in logs
            for match in log.get("matches", [])
            if match.get("severity") == "error" and match.get("key") in EARLY_FAILURE_LOG_KEYS
        ]
        if crash_artifacts:
            return {
                "status": "crashed",
                "profile": profile_brief(profile),
                "elapsedSeconds": round(time.time() - started_at, 3),
                "crashArtifacts": [str(path) for path in crash_artifacts],
                "readySignals": ready_signals,
                "logs": logs,
            }
        if ready_signals:
            return {
                "status": "ready",
                "profile": profile_brief(profile),
                "elapsedSeconds": round(time.time() - started_at, 3),
                "readySignals": ready_signals,
                "logs": logs,
            }
        if early_failures:
            return {
                "status": "failed",
                "profile": profile_brief(profile),
                "elapsedSeconds": round(time.time() - started_at, 3),
                "failures": early_failures[:20],
                "readySignals": ready_signals,
                "logs": logs,
            }

        last_logs = logs
        last_ready_signals = ready_signals
        if time.time() >= deadline:
            return {
                "status": "timeout",
                "profile": profile_brief(profile),
                "elapsedSeconds": round(time.time() - started_at, 3),
                "readySignals": last_ready_signals,
                "logs": last_logs,
            }
        time.sleep(poll_interval_seconds)


def list_profiles(ctx: DiagnosticContext) -> dict[str, Any]:
    data = load_profiles(ctx)
    return {
        "dataDir": data["dataDir"],
        "selectedProfileId": data["selectedProfileId"],
        "profiles": [profile_brief(profile) for profile in data.get("profiles", [])],
    }


def validate_mod_id(mod_id: str) -> str:
    mod_id = str(mod_id).strip()
    if not MOD_FILE_RE.match(mod_id):
        raise ValueError("mod_id must match Fabric id format: lowercase letters, digits, '_' or '-'")
    return mod_id


def validate_java_package(package: str) -> str:
    package = str(package).strip()
    if not re.match(r"^[a-z_][a-z0-9_]*(\.[a-z_][a-z0-9_]*)*$", package):
        raise ValueError("package must be a valid lowercase Java package name")
    return package


def java_class_name_for_mod_id(mod_id: str) -> str:
    parts = [part for part in re.split(r"[_-]+", mod_id) if part]
    name = "".join(part[:1].upper() + part[1:] for part in parts)
    if not name or not name[0].isalpha():
        name = "Codex" + name
    return name if name.endswith("Mod") else name + "Mod"


def title_for_mod_id(mod_id: str) -> str:
    return " ".join(part[:1].upper() + part[1:] for part in re.split(r"[_-]+", mod_id) if part)


def default_yarn_mappings(minecraft_version: str) -> str:
    if minecraft_version == "1.20.1":
        return "1.20.1+build.10"
    return f"{minecraft_version}+build.1"


def default_fabric_api_version(minecraft_version: str) -> str:
    if minecraft_version == "1.20.1":
        return "0.92.8+1.20.1"
    return ""


def write_scaffold_file(path: Path, content: str, replace: bool, created: list[str]) -> None:
    if path.exists() and not replace:
        raise FileExistsError(f"target file already exists: {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    created.append(str(path))


def scaffold_fabric_mod(
    project_dir: str,
    mod_id: str,
    name: str | None = None,
    package: str | None = None,
    minecraft_version: str = "1.20.1",
    loader_version: str = "0.15.11",
    fabric_api_version: str | None = None,
    yarn_mappings: str | None = None,
    mod_version: str = "0.1.0",
    replace: bool = False,
) -> dict[str, Any]:
    mod_id = validate_mod_id(mod_id)
    display_name = str(name).strip() if name else title_for_mod_id(mod_id)
    package = validate_java_package(package or f"dev.powermine.{mod_id.replace('-', '_')}")
    minecraft_version = str(minecraft_version).strip() or "1.20.1"
    fabric_api_version = fabric_api_version if fabric_api_version is not None else default_fabric_api_version(minecraft_version)
    yarn_mappings = yarn_mappings or default_yarn_mappings(minecraft_version)
    class_name = java_class_name_for_mod_id(mod_id)
    archives_base_name = mod_id.replace("_", "-") + "-mod"
    project = Path(project_dir).expanduser().resolve()
    if project.exists() and any(project.iterdir()) and not replace:
        raise FileExistsError(f"project directory is not empty: {project}")

    package_path = Path(*package.split("."))
    java_path = project / "src" / "main" / "java" / package_path / f"{class_name}.java"
    resource_root = project / "src" / "main" / "resources"
    lang_name = display_name + " Sample Item"
    created: list[str] = []

    write_scaffold_file(
        project / "settings.gradle",
        f"""pluginManagement {{
    repositories {{
        maven {{
            name = 'Fabric'
            url = 'https://maven.fabricmc.net/'
        }}
        gradlePluginPortal()
        mavenCentral()
    }}
}}

rootProject.name = '{archives_base_name}'
""",
        replace,
        created,
    )
    write_scaffold_file(
        project / "build.gradle",
        """plugins {
    id 'fabric-loom' version '1.10.5'
}

version = project.mod_version
group = project.maven_group

base {
    archivesName = project.archives_base_name
}

repositories {
    maven {
        name = 'Fabric'
        url = 'https://maven.fabricmc.net/'
    }
    mavenCentral()
}

dependencies {
    minecraft "com.mojang:minecraft:${project.minecraft_version}"
    mappings "net.fabricmc:yarn:${project.yarn_mappings}:v2"
    modImplementation "net.fabricmc:fabric-loader:${project.loader_version}"
    modImplementation "net.fabricmc.fabric-api:fabric-api:${project.fabric_version}"
}

java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(17)
    }
    withSourcesJar()
}

tasks.withType(JavaCompile).configureEach {
    options.release = 17
}

processResources {
    inputs.property 'version', project.version
    filesMatching('fabric.mod.json') {
        expand 'version': project.version
    }
}
""",
        replace,
        created,
    )
    write_scaffold_file(
        project / "gradle.properties",
        f"""org.gradle.jvmargs=-Xmx2G

minecraft_version={minecraft_version}
yarn_mappings={yarn_mappings}
loader_version={loader_version}
fabric_version={fabric_api_version}

mod_version={mod_version}
maven_group={package}
archives_base_name={archives_base_name}
""",
        replace,
        created,
    )
    write_scaffold_file(
        java_path,
        f"""package {package};

import net.fabricmc.api.ModInitializer;
import net.fabricmc.fabric.api.item.v1.FabricItemSettings;
import net.minecraft.item.Item;
import net.minecraft.registry.Registries;
import net.minecraft.registry.Registry;
import net.minecraft.util.Identifier;

public final class {class_name} implements ModInitializer {{
    public static final String MOD_ID = "{mod_id}";
    public static Item sampleItem;

    @Override
    public void onInitialize() {{
        sampleItem = Registry.register(
            Registries.ITEM,
            new Identifier(MOD_ID, "sample_item"),
            new Item(new FabricItemSettings())
        );
        System.out.println("[{display_name}] Registered " + MOD_ID + ":sample_item");
    }}
}}
""",
        replace,
        created,
    )

    fabric_metadata = {
        "schemaVersion": 1,
        "id": mod_id,
        "version": "${version}",
        "name": display_name,
        "description": "A minimal Fabric mod scaffolded by Power Mine Codex tools.",
        "authors": ["Codex"],
        "license": "MIT",
        "environment": "*",
        "entrypoints": {"main": [f"{package}.{class_name}"]},
        "depends": {
            "fabricloader": ">=0.15.0",
            "fabric-api": ">=0.92.0" if minecraft_version == "1.20.1" else "*",
            "minecraft": minecraft_version,
            "java": ">=17",
        },
    }
    write_scaffold_file(
        resource_root / "fabric.mod.json",
        json.dumps(fabric_metadata, ensure_ascii=False, indent=2) + "\n",
        replace,
        created,
    )
    write_scaffold_file(
        resource_root / "assets" / mod_id / "lang" / "en_us.json",
        json.dumps({f"item.{mod_id}.sample_item": lang_name}, ensure_ascii=False, indent=2) + "\n",
        replace,
        created,
    )
    write_scaffold_file(
        resource_root / "assets" / mod_id / "models" / "item" / "sample_item.json",
        json.dumps({"parent": "minecraft:item/generated", "textures": {"layer0": "minecraft:item/emerald"}}, indent=2) + "\n",
        replace,
        created,
    )
    write_scaffold_file(
        resource_root / "data" / mod_id / "recipes" / "sample_item.json",
        json.dumps(
            {
                "type": "minecraft:crafting_shapeless",
                "ingredients": [{"item": "minecraft:emerald"}],
                "result": {"item": f"{mod_id}:sample_item"},
            },
            indent=2,
        )
        + "\n",
        replace,
        created,
    )

    return {
        "ok": True,
        "projectDir": str(project),
        "modId": mod_id,
        "name": display_name,
        "package": package,
        "mainClass": f"{package}.{class_name}",
        "createdFiles": created,
        "buildCommand": f"gradle -p {project} build",
    }


def scaffold_forge_1710_mod(
    project_dir: str,
    mod_id: str,
    name: str | None = None,
    package: str | None = None,
    forge_version: str = "1.7.10-10.13.4.1614-1.7.10",
    mod_version: str = "0.1.0",
    replace: bool = False,
) -> dict[str, Any]:
    mod_id = validate_mod_id(mod_id)
    display_name = str(name).strip() if name else title_for_mod_id(mod_id)
    package = validate_java_package(package or f"dev.powermine.{mod_id.replace('-', '_')}")
    class_name = java_class_name_for_mod_id(mod_id)
    archives_base_name = mod_id.replace("_", "-") + "-forge-1.7.10"
    project = Path(project_dir).expanduser().resolve()
    if project.exists() and any(project.iterdir()) and not replace:
        raise FileExistsError(f"project directory is not empty: {project}")

    package_path = Path(*package.split("."))
    java_path = project / "src" / "main" / "java" / package_path / f"{class_name}.java"
    resource_root = project / "src" / "main" / "resources"
    build_script = project / "scripts" / "build-forge-1.7.10.sh"
    created: list[str] = []

    write_scaffold_file(
        build_script,
        f"""#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${{BASH_SOURCE[0]}}")/.." && pwd)"
DATA_DIR="${{POWER_MINE_DATA_DIR:-$HOME/.local/share/power-mine}}"
FORGE_VERSION="${{POWER_MINE_FORGE_1710_VERSION:-{forge_version}}}"
FORGE_JAR="${{POWER_MINE_FORGE_1710_JAR:-$DATA_DIR/minecraft/libraries/net/minecraftforge/forge/$FORGE_VERSION/forge-$FORGE_VERSION.jar}}"
MC_JAR="${{POWER_MINE_MC_1710_JAR:-$DATA_DIR/minecraft/versions/1.7.10/1.7.10.jar}}"
SRC_DIR="$ROOT_DIR/src/main/java"
RES_DIR="$ROOT_DIR/src/main/resources"
BUILD_DIR="$ROOT_DIR/build"
CLASS_DIR="$BUILD_DIR/classes"
JAR_PATH="$BUILD_DIR/libs/{archives_base_name}-{mod_version}.jar"
JAVAC_BIN="${{JAVAC:-javac}}"
JAR_BIN="${{JAR:-jar}}"

if [[ ! -f "$FORGE_JAR" ]]; then
  echo "Forge jar not found: $FORGE_JAR" >&2
  echo "Install or repair a Forge 1.7.10 profile first, or set POWER_MINE_FORGE_1710_JAR." >&2
  exit 1
fi

if [[ ! -f "$MC_JAR" ]]; then
  echo "Minecraft jar not found: $MC_JAR" >&2
  echo "Install or repair a Minecraft 1.7.10 profile first, or set POWER_MINE_MC_1710_JAR." >&2
  exit 1
fi

rm -rf "$CLASS_DIR"
mkdir -p "$CLASS_DIR" "$(dirname "$JAR_PATH")"

mapfile -t SOURCES < <(find "$SRC_DIR" -name '*.java' -print | sort)
if [[ ${{#SOURCES[@]}} -eq 0 ]]; then
  echo "No Java sources found in $SRC_DIR" >&2
  exit 1
fi

if "$JAVAC_BIN" --release 8 -version >/dev/null 2>&1; then
  "$JAVAC_BIN" --release 8 -cp "$FORGE_JAR:$MC_JAR" -d "$CLASS_DIR" "${{SOURCES[@]}}"
else
  "$JAVAC_BIN" -source 1.8 -target 1.8 -cp "$FORGE_JAR:$MC_JAR" -d "$CLASS_DIR" "${{SOURCES[@]}}"
fi

if [[ -d "$RES_DIR" ]]; then
  cp -R "$RES_DIR"/. "$CLASS_DIR"/
fi

"$JAR_BIN" cf "$JAR_PATH" -C "$CLASS_DIR" .
echo "$JAR_PATH"
""",
        replace,
        created,
    )
    build_script.chmod(0o755)

    write_scaffold_file(
        java_path,
        f"""package {package};

import cpw.mods.fml.common.Mod;
import cpw.mods.fml.common.event.FMLInitializationEvent;
import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.Method;

@Mod(
        modid = {class_name}.MOD_ID,
        name = "{display_name}",
        version = {class_name}.VERSION,
        acceptedMinecraftVersions = "[1.7.10]"
)
public final class {class_name} {{
    public static final String MOD_ID = "{mod_id}";
    public static final String VERSION = "{mod_version}";

    public static Object sampleItem;
    public static Object sampleBlock;

    @Mod.EventHandler
    public void init(FMLInitializationEvent event) throws Exception {{
        Class<?> itemClass = classByNameAny("net.minecraft.item.Item", "adb");
        Class<?> blockClass = classByNameAny("net.minecraft.block.Block", "aji");
        Class<?> materialClass = classByNameAny("net.minecraft.block.material.Material", "awt");
        Class<?> itemStackClass = classByNameAny("net.minecraft.item.ItemStack", "add");
        Class<?> gameRegistry = Class.forName("cpw.mods.fml.common.registry.GameRegistry");

        sampleItem = itemClass.getConstructor().newInstance();
        call(sampleItem, new String[]{{"setUnlocalizedName", "func_77655_b", "c"}}, new Class<?>[]{{String.class}}, MOD_ID + ".sample_item");
        call(sampleItem, new String[]{{"setTextureName", "func_111206_d", "f"}}, new Class<?>[]{{String.class}}, "minecraft:emerald");
        callStatic(gameRegistry, "registerItem", new Class<?>[]{{itemClass, String.class}}, sampleItem, "sample_item");

        Object rock = staticFieldAny(materialClass, "rock", "e", "d", "c");
        Constructor<?> blockConstructor = blockClass.getDeclaredConstructor(materialClass);
        blockConstructor.setAccessible(true);
        sampleBlock = blockConstructor.newInstance(rock);
        call(sampleBlock, new String[]{{"setBlockName", "func_149663_c", "c"}}, new Class<?>[]{{String.class}}, MOD_ID + ".sample_block");
        call(sampleBlock, new String[]{{"setBlockTextureName", "func_149658_d", "d"}}, new Class<?>[]{{String.class}}, "minecraft:stone");
        call(sampleBlock, new String[]{{"setHardness", "func_149711_c", "a"}}, new Class<?>[]{{float.class}}, 1.5F);
        call(sampleBlock, new String[]{{"setResistance", "func_149752_b", "b"}}, new Class<?>[]{{float.class}}, 6.0F);
        callStatic(gameRegistry, "registerBlock", new Class<?>[]{{blockClass, String.class}}, sampleBlock, "sample_block");

        Object emerald = callStatic(gameRegistry, "findItem", new Class<?>[]{{String.class, String.class}}, "minecraft", "emerald");
        Object stone = callStatic(gameRegistry, "findBlock", new Class<?>[]{{String.class, String.class}}, "minecraft", "stone");
        Object itemOutput = itemStackClass.getConstructor(itemClass).newInstance(sampleItem);
        Object blockOutput = itemStackClass.getConstructor(blockClass).newInstance(sampleBlock);
        callStatic(gameRegistry, "addShapelessRecipe", new Class<?>[]{{itemStackClass, Object[].class}}, itemOutput, new Object[]{{emerald}});
        callStatic(gameRegistry, "addRecipe", new Class<?>[]{{itemStackClass, Object[].class}}, blockOutput, new Object[]{{"SS", "SS", Character.valueOf('S'), stone}});
        System.out.println("[{display_name}] Registered " + MOD_ID + ":sample_item and " + MOD_ID + ":sample_block");
    }}

    private static Class<?> classByNameAny(String... names) throws ClassNotFoundException {{
        ClassNotFoundException last = null;
        for (String name : names) {{
            try {{
                return Class.forName(name);
            }} catch (ClassNotFoundException exc) {{
                last = exc;
            }}
        }}
        throw last == null ? new ClassNotFoundException("no class names provided") : last;
    }}

    private static Object call(Object target, String[] names, Class<?>[] types, Object... args) throws Exception {{
        Class<?> current = target.getClass();
        while (current != null) {{
            for (String name : names) {{
                try {{
                    Method method = current.getDeclaredMethod(name, types);
                    method.setAccessible(true);
                    return method.invoke(target, args);
                }} catch (NoSuchMethodException ignored) {{
                }}
            }}
            current = current.getSuperclass();
        }}
        throw new NoSuchMethodException(names[0]);
    }}

    private static Object callStatic(Class<?> owner, String name, Class<?>[] types, Object... args) throws Exception {{
        Method method = owner.getMethod(name, types);
        method.setAccessible(true);
        return method.invoke(null, args);
    }}

    private static Object staticFieldAny(Class<?> owner, String... names) throws Exception {{
        for (String name : names) {{
            try {{
                Field field = owner.getField(name);
                field.setAccessible(true);
                Object value = field.get(null);
                if (value != null) {{
                    return value;
                }}
            }} catch (NoSuchFieldException ignored) {{
            }}
        }}
        for (Field field : owner.getFields()) {{
            if (owner.isAssignableFrom(field.getType())) {{
                field.setAccessible(true);
                Object value = field.get(null);
                if (value != null) {{
                    return value;
                }}
            }}
        }}
        throw new NoSuchFieldException(owner.getName());
    }}
}}
""",
        replace,
        created,
    )

    mcmod_info = [
        {
            "modid": mod_id,
            "name": display_name,
            "description": "A minimal Forge 1.7.10 mod scaffolded by Power Mine Codex tools.",
            "version": mod_version,
            "mcversion": "1.7.10",
            "url": "",
            "updateUrl": "",
            "authorList": ["Codex"],
            "credits": "",
            "logoFile": "",
            "screenshots": [],
            "dependencies": [],
        }
    ]
    write_scaffold_file(
        resource_root / "mcmod.info",
        json.dumps(mcmod_info, ensure_ascii=False, indent=2) + "\n",
        replace,
        created,
    )
    write_scaffold_file(
        resource_root / "assets" / mod_id / "lang" / "en_US.lang",
        f"""item.{mod_id}.sample_item.name={display_name} Sample Item
tile.{mod_id}.sample_block.name={display_name} Sample Block
""",
        replace,
        created,
    )
    write_scaffold_file(
        project / "README.md",
        f"""# {display_name}

Minimal Forge 1.7.10 mod scaffold for Power Mine/Codex runtime checks.

Build:

```bash
scripts/build-forge-1.7.10.sh
```

Default runtime smoke targets:

- item: `{mod_id}:sample_item`
- block: `{mod_id}:sample_block`
- recipe: `minecraft:emerald` -> `{mod_id}:sample_item`
""",
        replace,
        created,
    )

    return {
        "ok": True,
        "projectDir": str(project),
        "modId": mod_id,
        "name": display_name,
        "package": package,
        "mainClass": f"{package}.{class_name}",
        "createdFiles": created,
        "buildCommand": str(build_script),
        "jarPath": str(project / "build" / "libs" / f"{archives_base_name}-{mod_version}.jar"),
        "defaultSmoke": {
            "giveItem": f"{mod_id}:sample_item",
            "recipeItems": ["minecraft:emerald"],
            "expectedOutput": f"{mod_id}:sample_item",
            "block": f"{mod_id}:sample_block",
        },
    }


def json_dump(data: Any, pretty: bool = False) -> str:
    return json.dumps(data, ensure_ascii=False, indent=2 if pretty else None)


TOOLS = [
    {
        "name": "list_profiles",
        "description": "List Power Mine launcher profiles from the local data directory.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "diagnose_profile",
        "description": "Inspect a Power Mine profile, its installed mods, and recent game logs.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
                "include_logs": {"type": "boolean", "description": "Analyze recent logs and crash reports.", "default": True},
                "max_log_bytes": {"type": "integer", "description": "Maximum bytes to read from each log.", "default": MAX_LOG_BYTES},
                "log_scope": {
                    "type": "string",
                    "description": "Which logs to analyze: recent history or only files from the latest launcher run.",
                    "enum": ["recent", "latest_run"],
                    "default": "recent",
                },
            },
        },
    },
    {
        "name": "wait_profile_ready",
        "description": "Wait until a launched profile reaches client-ready log signals, or report a crash/failure.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
                "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait.", "default": 90},
                "poll_interval_seconds": {"type": "number", "description": "Seconds between log polls.", "default": 2},
                "max_log_bytes": {"type": "integer", "description": "Maximum bytes to read from each log.", "default": MAX_LOG_BYTES},
            },
        },
    },
    {
        "name": "create_profile",
        "description": "Create a Power Mine profile through the launcher headless command.",
        "inputSchema": {
            "type": "object",
            "required": ["name"],
            "properties": {
                "name": {"type": "string", "description": "Profile display name."},
                "minecraft_version": {"type": "string", "description": "Minecraft version.", "default": "1.20.1"},
                "loader": {"type": "string", "description": "Loader type.", "default": "fabric"},
                "loader_version": {"type": "string", "description": "Loader version.", "default": "latest"},
                "game_dir": {"type": "string", "description": "Optional profile game directory."},
                "min_memory": {"type": "integer", "description": "Minimum memory in MB.", "default": 1024},
                "max_memory": {"type": "integer", "description": "Maximum memory in MB.", "default": 4096},
                "install": {"type": "boolean", "description": "Install the profile immediately after creating it.", "default": False},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "install_java_runtime",
        "description": "Install a managed Eclipse Temurin Java runtime through the launcher headless command.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "version": {"type": "integer", "description": "Java major version.", "default": 21},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "scaffold_fabric_mod",
        "description": "Create a minimal buildable Fabric mod project for Codex-driven testing.",
        "inputSchema": {
            "type": "object",
            "required": ["project_dir", "mod_id"],
            "properties": {
                "project_dir": {"type": "string", "description": "Directory where the mod project should be created."},
                "mod_id": {"type": "string", "description": "Fabric mod id, lowercase with digits, '_' or '-'."},
                "name": {"type": "string", "description": "Human-readable mod name."},
                "package": {"type": "string", "description": "Java package. Defaults under dev.powermine."},
                "minecraft_version": {"type": "string", "description": "Minecraft version.", "default": "1.20.1"},
                "loader_version": {"type": "string", "description": "Fabric Loader version.", "default": "0.15.11"},
                "fabric_api_version": {"type": "string", "description": "Fabric API version.", "default": "0.92.8+1.20.1"},
                "yarn_mappings": {"type": "string", "description": "Yarn mappings version.", "default": "1.20.1+build.10"},
                "mod_version": {"type": "string", "description": "Initial mod version.", "default": "0.1.0"},
                "replace": {"type": "boolean", "description": "Overwrite generated files if they already exist.", "default": False},
            },
        },
    },
    {
        "name": "scaffold_forge_1710_mod",
        "description": "Create a minimal buildable Forge 1.7.10 mod project for Codex-driven testing.",
        "inputSchema": {
            "type": "object",
            "required": ["project_dir", "mod_id"],
            "properties": {
                "project_dir": {"type": "string", "description": "Directory where the mod project should be created."},
                "mod_id": {"type": "string", "description": "Forge mod id, lowercase with digits, '_' or '-'."},
                "name": {"type": "string", "description": "Human-readable mod name."},
                "package": {"type": "string", "description": "Java package. Defaults under dev.powermine."},
                "forge_version": {"type": "string", "description": "Forge 1.7.10 version.", "default": "1.7.10-10.13.4.1614-1.7.10"},
                "mod_version": {"type": "string", "description": "Initial mod version.", "default": "0.1.0"},
                "replace": {"type": "boolean", "description": "Overwrite generated files if they already exist.", "default": False},
            },
        },
    },
    {
        "name": "build_mod_project",
        "description": "Build a local Minecraft mod project and return the newest jar artifact.",
        "inputSchema": {
            "type": "object",
            "required": ["project_dir"],
            "properties": {
                "project_dir": {"type": "string", "description": "Mod project directory."},
                "build_command": {"type": "string", "description": "Optional command, for example './gradlew build'."},
                "artifact_glob": {"type": "string", "description": "Optional project-relative artifact glob, for example 'build/libs/*.jar'."},
                "timeout_seconds": {"type": "integer", "description": "Build timeout in seconds.", "default": 300},
            },
        },
    },
    {
        "name": "diagnose_mod",
        "description": "Inspect a single Minecraft mod jar and optionally compare it with a Power Mine profile.",
        "inputSchema": {
            "type": "object",
            "required": ["jar_path"],
            "properties": {
                "jar_path": {"type": "string", "description": "Path to a .jar or .jar.disabled mod file."},
                "profile_id": {"type": "string", "description": "Optional profile id for loader/version compatibility checks."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "diagnose_mod_content",
        "description": "Statically validate a mod jar's item/block models, textures, blockstates, and crafting recipes.",
        "inputSchema": {
            "type": "object",
            "required": ["jar_path"],
            "properties": {
                "jar_path": {"type": "string", "description": "Path to a .jar or .jar.disabled mod file."},
            },
        },
    },
    {
        "name": "import_profile_mod",
        "description": "Copy a readable mod jar into a Power Mine profile's mods folder.",
        "inputSchema": {
            "type": "object",
            "required": ["jar_path"],
            "properties": {
                "jar_path": {"type": "string", "description": "Path to the source .jar mod file."},
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
                "file_name": {"type": "string", "description": "Optional target file name in the profile mods folder."},
                "enabled": {"type": "boolean", "description": "Install enabled as .jar, or disabled as .jar.disabled.", "default": True},
                "replace": {"type": "boolean", "description": "Replace an existing mod file with the same name.", "default": False},
            },
        },
    },
    {
        "name": "set_profile_mod_enabled",
        "description": "Enable or disable an installed profile mod by renaming .jar/.jar.disabled.",
        "inputSchema": {
            "type": "object",
            "required": ["file_name", "enabled"],
            "properties": {
                "file_name": {"type": "string", "description": "Installed mod file name."},
                "enabled": {"type": "boolean", "description": "Whether the mod should be enabled."},
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "delete_profile_mod",
        "description": "Delete an installed profile mod file.",
        "inputSchema": {
            "type": "object",
            "required": ["file_name"],
            "properties": {
                "file_name": {"type": "string", "description": "Installed mod file name."},
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "build_agent_mod",
        "description": "Build a Power Mine runtime agent mod jar.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "target": {
                    "type": "string",
                    "description": "Agent target: fabric-1.20.1, forge-1.7.10, or forge-1.12.2. Defaults to fabric-1.20.1.",
                    "default": FABRIC_AGENT_TARGET,
                },
            },
        },
    },
    {
        "name": "install_agent_mod",
        "description": "Build and install the matching Power Mine runtime agent mod into a profile's mods folder.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
                "target": {
                    "type": "string",
                    "description": "Optional explicit agent target. By default MCP selects from profile loader/version.",
                },
                "build": {"type": "boolean", "description": "Build the agent jar before installing.", "default": True},
                "replace": {"type": "boolean", "description": "Replace an existing installed agent jar.", "default": True},
            },
        },
    },
    {
        "name": "install_profile",
        "description": "Install a Power Mine profile through the launcher's headless command.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "repair_profile",
        "description": "Repair a Power Mine profile through the launcher's headless command.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
            },
        },
    },
    {
        "name": "launch_profile",
        "description": "Launch a Power Mine profile through the launcher's headless command.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
                "quick_play_singleplayer": {"type": "string", "description": "Optional existing singleplayer world name to open via Minecraft Quick Play."},
                "keep_pause_on_lost_focus": {"type": "boolean", "description": "Keep Minecraft's pauseOnLostFocus setting instead of forcing it false before launch.", "default": False},
            },
        },
    },
    {
        "name": "agent_smoke_test",
        "description": "Run one structured static+runtime smoke test through the Power Mine profile and in-game agent.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
                "jar_path": {"type": "string", "description": "Optional mod jar to inspect before runtime checks."},
                "launch": {"type": "boolean", "description": "Launch the profile before waiting for the agent.", "default": False},
                "quick_play_singleplayer": {"type": "string", "description": "Existing singleplayer world name to open via Minecraft Quick Play."},
                "world_name": {"type": "string", "description": "World name to open through the in-game agent after launch/readiness."},
                "create_world": {"type": "boolean", "description": "Ask the agent to create the world when it is missing and supported.", "default": False},
                "world_gamemode": {"type": "string", "description": "Game mode for agent-created worlds.", "default": "creative"},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
                "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for a loaded agent world.", "default": 120},
                "poll_interval_seconds": {"type": "number", "description": "Seconds between agent polls.", "default": 2},
                "hotbar_slot": {"type": "integer", "description": "Hotbar slot to select before held-item render check.", "default": 0},
                "give_item": {"type": "string", "description": "Optional item id to put into the player's inventory before held-item render check."},
                "give_count": {"type": "integer", "description": "Item count for give_item.", "default": 1},
                "give_slot": {"type": "integer", "description": "Inventory slot for give_item. Defaults to hotbar slot 0.", "default": 0},
                "give_select": {"type": "boolean", "description": "Select give_slot after giving the item when it is a hotbar slot.", "default": True},
                "give_replace": {"type": "boolean", "description": "Replace the target slot when it is occupied.", "default": True},
                "release_input": {"type": "boolean", "description": "Ask the agent to disable pause-on-lost-focus and release mouse input after entering the world.", "default": True},
                "check_held_item": {"type": "boolean", "description": "Run held item render check. Fabric reports baked model state; Forge 1.7.10 reports a screenshot visual probe.", "default": True},
                "take_screenshot": {"type": "boolean", "description": "Capture a framebuffer screenshot.", "default": True},
                "recipe_items": {
                    "type": "array",
                    "description": "Crafting grid entries, row-major. Defaults to a 1x1 oak_log -> oak_planks check.",
                },
                "recipe_width": {"type": "integer", "description": "Crafting grid width 1-3.", "default": 1},
                "recipe_height": {"type": "integer", "description": "Crafting grid height 1-3.", "default": 1},
                "expected_output": {"type": "string", "description": "Optional item id expected as recipe output. Defaults to minecraft:oak_planks only for the default oak_log recipe."},
                "recipe_id": {"type": "string", "description": "Optional recipe id expected to match."},
                "require_inventory": {"type": "boolean", "description": "Require current inventory to contain recipe inputs.", "default": False},
                "craft_recipe": {"type": "boolean", "description": "After recipe_check, give inputs if requested and perform a real inventory craft through the agent.", "default": False},
                "craft_give_inputs": {"type": "boolean", "description": "Give the recipe inputs to inventory slots before craft_recipe.", "default": True},
                "crafts": {"type": "integer", "description": "Number of craft iterations for craft_recipe.", "default": 1},
                "block": {"type": "string", "description": "Block id to place and render-check.", "default": "minecraft:stone"},
                "block_x": {"type": "integer", "description": "Optional block x. Defaults beside the player."},
                "block_y": {"type": "integer", "description": "Optional block y. Defaults beside the player."},
                "block_z": {"type": "integer", "description": "Optional block z. Defaults beside the player."},
                "check_use_item": {"type": "boolean", "description": "Run an item right-click interaction check when give_item is present.", "default": True},
                "check_use_block": {"type": "boolean", "description": "Run a block right-click interaction check after the smoke block is placed.", "default": True},
                "cleanup": {"type": "boolean", "description": "Restore or clear the smoke-test block position after the check.", "default": True},
                "include_logs": {"type": "boolean", "description": "Analyze latest-run logs after runtime checks.", "default": True},
            },
        },
    },
    {
        "name": "mod_dev_loop",
        "description": "Build a mod project, import its jar into a profile, install the matching agent, launch Minecraft, and run a runtime smoke test.",
        "inputSchema": {
            "type": "object",
            "required": ["project_dir"],
            "properties": {
                "project_dir": {"type": "string", "description": "Mod project directory."},
                "profile_id": {"type": "string", "description": "Profile id. Defaults to the selected profile."},
                "data_dir": {"type": "string", "description": "Optional Power Mine data directory."},
                "build": {"type": "boolean", "description": "Build the project before importing.", "default": True},
                "build_command": {"type": "string", "description": "Optional build command, for example './gradlew build'."},
                "artifact_glob": {"type": "string", "description": "Optional project-relative artifact glob."},
                "jar_path": {"type": "string", "description": "Optional existing jar path to use instead of the discovered build artifact."},
                "import_mod": {"type": "boolean", "description": "Import the jar into the profile mods folder.", "default": True},
                "import_file_name": {"type": "string", "description": "Optional target file name in the profile mods folder."},
                "replace_mod": {"type": "boolean", "description": "Replace an existing installed mod file.", "default": True},
                "install_agent": {"type": "boolean", "description": "Build/install the matching Power Mine runtime agent before launch.", "default": True},
                "launch": {"type": "boolean", "description": "Launch the profile before runtime checks.", "default": True},
                "keep_running": {"type": "boolean", "description": "Keep Minecraft running after the dev-loop smoke test.", "default": False},
                "quick_play_singleplayer": {"type": "string", "description": "Existing singleplayer world name to open via Minecraft Quick Play."},
                "world_name": {"type": "string", "description": "World name to open/create through the agent.", "default": "Power Mine Codex Dev Test"},
                "create_world": {"type": "boolean", "description": "Ask the agent to create the world if missing.", "default": True},
                "world_gamemode": {"type": "string", "description": "Game mode for agent-created worlds.", "default": "creative"},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
                "timeout_seconds": {"type": "integer", "description": "Runtime timeout in seconds.", "default": 120},
                "poll_interval_seconds": {"type": "number", "description": "Seconds between runtime polls.", "default": 2},
                "use_sample_targets": {"type": "boolean", "description": "Infer sample_item/sample_block targets from the jar mod id.", "default": True},
                "give_item": {"type": "string", "description": "Item id to put into the player's hotbar."},
                "give_count": {"type": "integer", "description": "Item count for give_item.", "default": 1},
                "give_slot": {"type": "integer", "description": "Inventory slot for give_item.", "default": 0},
                "recipe_items": {"type": "array", "description": "Crafting grid entries, row-major."},
                "recipe_width": {"type": "integer", "description": "Crafting grid width 1-3.", "default": 1},
                "recipe_height": {"type": "integer", "description": "Crafting grid height 1-3.", "default": 1},
                "expected_output": {"type": "string", "description": "Expected recipe output item id."},
                "craft_recipe": {"type": "boolean", "description": "Perform a real inventory craft after recipe_check in the runtime smoke test.", "default": True},
                "craft_give_inputs": {"type": "boolean", "description": "Give the recipe inputs to inventory slots before the runtime craft.", "default": True},
                "crafts": {"type": "integer", "description": "Number of craft iterations for the runtime craft.", "default": 1},
                "block": {"type": "string", "description": "Block id to place and render-check."},
                "cleanup": {"type": "boolean", "description": "Restore or clear the smoke-test block position.", "default": True},
                "include_logs": {"type": "boolean", "description": "Analyze latest-run logs after runtime checks.", "default": True},
                "check_held_item": {"type": "boolean", "description": "Run held item render/visual check.", "default": True},
                "take_screenshot": {"type": "boolean", "description": "Capture a framebuffer screenshot.", "default": True},
                "check_use_item": {"type": "boolean", "description": "Run an item right-click interaction check when give_item is present.", "default": True},
                "check_use_block": {"type": "boolean", "description": "Run a block right-click interaction check after the smoke block is placed.", "default": True},
            },
        },
    },
    {
        "name": "agent_world_open",
        "description": "Open or create a singleplayer world through the running in-game agent.",
        "inputSchema": {
            "type": "object",
            "required": ["world"],
            "properties": {
                "world": {"type": "string", "description": "World save name to open or create."},
                "create": {"type": "boolean", "description": "Create the world if it is missing and the agent supports creation.", "default": True},
                "gamemode": {"type": "string", "description": "Game mode for a newly created world.", "default": "creative"},
                "seed": {"type": "integer", "description": "Optional world seed for newly created worlds."},
                "display_name": {"type": "string", "description": "Optional world display name."},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
                "timeout_seconds": {"type": "integer", "description": "HTTP timeout for slow legacy world creation.", "default": 90},
            },
        },
    },
    {
        "name": "agent_give_item",
        "description": "Put an item stack into a player inventory slot through the in-game agent.",
        "inputSchema": {
            "type": "object",
            "required": ["item"],
            "properties": {
                "item": {"type": "string", "description": "Item id to give, for example minecraft:diamond."},
                "count": {"type": "integer", "description": "Item count.", "default": 1},
                "slot": {"type": "integer", "description": "Inventory slot 0-40. Hotbar slots are 0-8.", "default": 0},
                "select": {"type": "boolean", "description": "Select this slot when it is a hotbar slot.", "default": True},
                "replace": {"type": "boolean", "description": "Replace the target slot when it is occupied.", "default": True},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_wait_ticks",
        "description": "Wait for the loaded Minecraft world to advance by a number of game ticks.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "ticks": {"type": "integer", "description": "Game ticks to wait for.", "default": 20},
                "timeout_seconds": {"type": "integer", "description": "Maximum wall-clock seconds to wait.", "default": 10},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_use_item",
        "description": "Use/right-click the currently held item through the running in-game agent.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "hand": {"type": "string", "description": "main or offhand. Forge 1.7.10 supports only main.", "default": "main"},
                "slot": {"type": "integer", "description": "Optional hotbar slot 0-8 to select before using the item."},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_use_block",
        "description": "Use/right-click a block position through the running in-game agent.",
        "inputSchema": {
            "type": "object",
            "required": ["x", "y", "z"],
            "properties": {
                "x": {"type": "integer", "description": "Block x."},
                "y": {"type": "integer", "description": "Block y."},
                "z": {"type": "integer", "description": "Block z."},
                "hand": {"type": "string", "description": "main or offhand. Forge 1.7.10 supports only main.", "default": "main"},
                "side": {"type": "string", "description": "Clicked side: up, down, north, south, west, or east.", "default": "up"},
                "slot": {"type": "integer", "description": "Optional hotbar slot 0-8 to select before using the block."},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_interaction_check",
        "description": "Run a structured item or block right-click check and report observed screen, stack, block, and tick changes.",
        "inputSchema": {
            "type": "object",
            "required": ["mode"],
            "properties": {
                "mode": {"type": "string", "description": "Interaction mode: item or block."},
                "x": {"type": "integer", "description": "Block x for block mode. Defaults beside the player."},
                "y": {"type": "integer", "description": "Block y for block mode. Defaults beside the player."},
                "z": {"type": "integer", "description": "Block z for block mode. Defaults beside the player."},
                "block": {"type": "string", "description": "Optional block id to place before block mode interaction."},
                "item": {"type": "string", "description": "Optional item id to give/select before the interaction."},
                "count": {"type": "integer", "description": "Item count when item is supplied.", "default": 1},
                "slot": {"type": "integer", "description": "Optional hotbar slot 0-8 to select/use.", "default": 0},
                "hand": {"type": "string", "description": "main or offhand. Forge 1.7.10 supports only main.", "default": "main"},
                "side": {"type": "string", "description": "Clicked block side.", "default": "up"},
                "wait_ticks": {"type": "integer", "description": "Game ticks to wait after the interaction.", "default": 2},
                "timeout_seconds": {"type": "integer", "description": "Wait timeout in seconds.", "default": 10},
                "require_effect": {"type": "boolean", "description": "Require at least one observed effect: used, screen opened, stack changed, or block changed.", "default": False},
                "expect_used": {"type": "boolean", "description": "Optional expected used/accepted result."},
                "expect_screen_open": {"type": "boolean", "description": "Optional expected screen-open result."},
                "expect_block_changed": {"type": "boolean", "description": "Optional expected block-change result."},
                "expect_stack_changed": {"type": "boolean", "description": "Optional expected stack-change result."},
                "cleanup": {"type": "boolean", "description": "If block was placed by this check, restore/clear it afterward.", "default": False},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_health",
        "description": "Check the Power Mine in-game agent HTTP server.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_capabilities",
        "description": "Read bridge protocol metadata and supported runtime capabilities from the running in-game agent.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_state",
        "description": "Read loaded-world, player, crosshair target, and inventory summary from the in-game agent.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_inventory",
        "description": "Read all player inventory slots from the in-game agent.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_held_item_render",
        "description": "Check the selected hand item stack and render evidence from the running client. Fabric reports baked model state; Forge 1.7.10 reports a screenshot visual probe.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "hand": {"type": "string", "description": "main or offhand.", "default": "main"},
                "delay_ms": {"type": "integer", "description": "Delay before screenshot visual probes, used by legacy agents.", "default": 250},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_release_input",
        "description": "Ask the running agent to disable pause-on-lost-focus and release captured mouse input.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_block_render",
        "description": "Check block render evidence for a block at a world position. Fabric reports baked model state; Forge 1.7.10 can turn the camera to the block and return a screenshot visual probe.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "x": {"type": "integer", "description": "Optional block x; defaults to player block x."},
                "y": {"type": "integer", "description": "Optional block y; defaults to player block y."},
                "z": {"type": "integer", "description": "Optional block z; defaults to player block z."},
                "look_at": {"type": "boolean", "description": "Ask legacy visual-probe agents to turn the camera toward the block before capturing.", "default": True},
                "delay_ms": {"type": "integer", "description": "Delay after camera movement before screenshot visual probes.", "default": 350},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_screenshot",
        "description": "Capture the current Minecraft framebuffer to a PNG file and return basic nonblank stats.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_camera_look",
        "description": "Rotate the running client camera/head by yaw/pitch or toward x/y/z, optionally waiting and capturing a screenshot.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "yaw": {"type": "number", "description": "Absolute camera yaw in degrees. Use with pitch."},
                "pitch": {"type": "number", "description": "Absolute camera pitch in degrees. Use with yaw."},
                "x": {"type": "number", "description": "Target x coordinate to look at. Use with y and z."},
                "y": {"type": "number", "description": "Target y coordinate to look at. Use with x and z."},
                "z": {"type": "number", "description": "Target z coordinate to look at. Use with x and y."},
                "center": {"type": "boolean", "description": "Add 0.5 to x/y/z target coordinates to look at block center.", "default": True},
                "screenshot": {"type": "boolean", "description": "Capture a framebuffer screenshot after rotating.", "default": False},
                "delay_ms": {"type": "integer", "description": "Delay after camera movement before screenshot. Defaults to 250 when screenshot=true, otherwise 0."},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_recipe_check",
        "description": "Ask Minecraft RecipeManager whether a crafting grid matches and optionally whether inventory contains the inputs.",
        "inputSchema": {
            "type": "object",
            "required": ["items"],
            "properties": {
                "items": {
                    "type": "array",
                    "description": "Crafting grid entries, row-major. Use item ids, objects with id/count, empty strings, or minecraft:air.",
                },
                "width": {"type": "integer", "description": "Crafting grid width 1-3.", "default": 3},
                "height": {"type": "integer", "description": "Crafting grid height 1-3.", "default": 3},
                "expected_output": {"type": "string", "description": "Optional item id expected as recipe output."},
                "recipe_id": {"type": "string", "description": "Optional recipe id expected to match."},
                "require_inventory": {"type": "boolean", "description": "Require current player inventory to contain grid inputs.", "default": False},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_craft_recipe",
        "description": "Craft a matching grid through the running Minecraft agent, consuming player inventory inputs and inserting the output stack.",
        "inputSchema": {
            "type": "object",
            "required": ["items"],
            "properties": {
                "items": {
                    "type": "array",
                    "description": "Crafting grid entries, row-major. Use item ids, objects with id/count/damage, empty strings, or minecraft:air.",
                },
                "width": {"type": "integer", "description": "Crafting grid width 1-3.", "default": 3},
                "height": {"type": "integer", "description": "Crafting grid height 1-3.", "default": 3},
                "expected_output": {"type": "string", "description": "Optional item id expected as recipe output."},
                "crafts": {"type": "integer", "description": "Number of craft iterations to perform.", "default": 1},
                "require_inventory": {"type": "boolean", "description": "Require player inventory to contain recipe inputs before crafting.", "default": True},
                "consume": {"type": "boolean", "description": "Consume recipe inputs from the player inventory.", "default": True},
                "insert_output": {"type": "boolean", "description": "Insert the crafted output into player inventory.", "default": True},
                "output_slot": {"type": "integer", "description": "Optional inventory slot for the output. Defaults to the first empty main/hotbar slot."},
                "replace_output": {"type": "boolean", "description": "Replace output_slot when occupied.", "default": False},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_world_snapshot",
        "description": "Read block ids around the player or a given block position from the in-game agent.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "radius": {"type": "integer", "description": "Snapshot radius, clamped by the agent.", "default": 2},
                "include_air": {"type": "boolean", "description": "Include air blocks.", "default": False},
                "x": {"type": "integer", "description": "Optional center x."},
                "y": {"type": "integer", "description": "Optional center y."},
                "z": {"type": "integer", "description": "Optional center z."},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_select_hotbar_slot",
        "description": "Select a player hotbar slot through the in-game agent.",
        "inputSchema": {
            "type": "object",
            "required": ["slot"],
            "properties": {
                "slot": {"type": "integer", "description": "Hotbar slot 0-8."},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_place_block",
        "description": "Place/set a block in the loaded integrated singleplayer world through the in-game agent.",
        "inputSchema": {
            "type": "object",
            "required": ["x", "y", "z"],
            "properties": {
                "x": {"type": "integer", "description": "Block x."},
                "y": {"type": "integer", "description": "Block y."},
                "z": {"type": "integer", "description": "Block z."},
                "block": {"type": "string", "description": "Block id to set.", "default": "minecraft:stone"},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
    {
        "name": "agent_break_block",
        "description": "Break a block in the loaded integrated singleplayer world through the in-game agent.",
        "inputSchema": {
            "type": "object",
            "required": ["x", "y", "z"],
            "properties": {
                "x": {"type": "integer", "description": "Block x."},
                "y": {"type": "integer", "description": "Block y."},
                "z": {"type": "integer", "description": "Block z."},
                "drop": {"type": "boolean", "description": "Whether the broken block should drop items.", "default": False},
                "port": {"type": "integer", "description": "Agent localhost port.", "default": DEFAULT_AGENT_PORT},
                "token": {"type": "string", "description": "Optional agent bearer token."},
            },
        },
    },
]


def read_mcp_message(stream: Any) -> dict[str, Any] | None:
    first = stream.readline()
    while first == b"\r\n" or first == b"\n":
        first = stream.readline()
    if not first:
        return None
    if first.lstrip().startswith(b"{"):
        return json.loads(first.decode("utf-8"))

    headers: dict[str, str] = {}
    line = first
    while line not in (b"\r\n", b"\n", b""):
        name, _, value = line.decode("utf-8").partition(":")
        headers[name.lower()] = value.strip()
        line = stream.readline()
    length = int(headers.get("content-length", "0"))
    if length <= 0:
        return None
    return json.loads(stream.read(length).decode("utf-8"))


def write_mcp_message(payload: dict[str, Any]) -> None:
    raw = json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    sys.stdout.buffer.write(raw)
    sys.stdout.buffer.write(b"\n")
    sys.stdout.buffer.flush()


def mcp_result(request_id: Any, result: Any) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def mcp_error(request_id: Any, code: int, message: str) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}


def log_mcp_exception(message: str) -> None:
    print(f"power-mine MCP: {message}", file=sys.stderr, flush=True)


def call_tool(name: str, arguments: dict[str, Any]) -> dict[str, Any]:
    ctx = context_from_args(arguments.get("data_dir"))
    if name == "list_profiles":
        data = list_profiles(ctx)
    elif name == "diagnose_profile":
        data = diagnose_profile(
            ctx,
            arguments.get("profile_id"),
            bool(arguments.get("include_logs", True)),
            int(arguments.get("max_log_bytes", MAX_LOG_BYTES)),
            arguments.get("log_scope", "recent"),
        )
    elif name == "wait_profile_ready":
        data = wait_profile_ready(
            ctx,
            arguments.get("profile_id"),
            int(arguments.get("timeout_seconds", 90)),
            float(arguments.get("poll_interval_seconds", 2)),
            int(arguments.get("max_log_bytes", MAX_LOG_BYTES)),
        )
    elif name == "create_profile":
        data = create_profile(
            ctx,
            arguments["name"],
            arguments.get("minecraft_version", "1.20.1"),
            arguments.get("loader", "fabric"),
            arguments.get("loader_version", "latest"),
            arguments.get("game_dir"),
            int(arguments.get("min_memory", 1024)),
            int(arguments.get("max_memory", 4096)),
            bool(arguments.get("install", False)),
        )
    elif name == "scaffold_fabric_mod":
        data = scaffold_fabric_mod(
            arguments["project_dir"],
            arguments["mod_id"],
            arguments.get("name"),
            arguments.get("package"),
            arguments.get("minecraft_version", "1.20.1"),
            arguments.get("loader_version", "0.15.11"),
            arguments.get("fabric_api_version"),
            arguments.get("yarn_mappings"),
            arguments.get("mod_version", "0.1.0"),
            bool(arguments.get("replace", False)),
        )
    elif name == "scaffold_forge_1710_mod":
        data = scaffold_forge_1710_mod(
            arguments["project_dir"],
            arguments["mod_id"],
            arguments.get("name"),
            arguments.get("package"),
            arguments.get("forge_version", "1.7.10-10.13.4.1614-1.7.10"),
            arguments.get("mod_version", "0.1.0"),
            bool(arguments.get("replace", False)),
        )
    elif name == "build_mod_project":
        data = build_mod_project(
            arguments["project_dir"],
            arguments.get("build_command"),
            arguments.get("artifact_glob"),
            int(arguments.get("timeout_seconds", 300)),
        )
    elif name == "diagnose_mod":
        profile = find_profile(ctx, arguments.get("profile_id")) if arguments.get("profile_id") else None
        data = inspect_mod(arguments["jar_path"], profile)
    elif name == "install_java_runtime":
        data = install_java_runtime(ctx, int(arguments.get("version", 21)))
    elif name == "diagnose_mod_content":
        data = diagnose_mod_content(arguments["jar_path"])
    elif name == "import_profile_mod":
        data = import_profile_mod(
            ctx,
            arguments.get("profile_id"),
            arguments["jar_path"],
            arguments.get("file_name"),
            bool(arguments.get("enabled", True)),
            bool(arguments.get("replace", False)),
        )
    elif name == "set_profile_mod_enabled":
        data = set_profile_mod_enabled(ctx, arguments.get("profile_id"), arguments["file_name"], bool(arguments["enabled"]))
    elif name == "delete_profile_mod":
        data = delete_profile_mod(ctx, arguments.get("profile_id"), arguments["file_name"])
    elif name == "build_agent_mod":
        data = build_agent_mod(arguments.get("target"))
    elif name == "install_agent_mod":
        data = install_agent_mod(
            ctx,
            arguments.get("profile_id"),
            bool(arguments.get("build", True)),
            bool(arguments.get("replace", True)),
            arguments.get("target"),
        )
    elif name == "install_profile":
        data = run_power_mine_headless(ctx, "install-profile", arguments.get("profile_id"))
    elif name == "repair_profile":
        data = run_power_mine_headless(ctx, "repair-profile", arguments.get("profile_id"))
    elif name == "launch_profile":
        data = launch_profile(
            ctx,
            arguments.get("profile_id"),
            arguments.get("quick_play_singleplayer"),
            bool(arguments.get("keep_pause_on_lost_focus", False)),
        )
    elif name == "agent_smoke_test":
        data = agent_smoke_test(
            ctx,
            arguments.get("profile_id"),
            arguments.get("jar_path"),
            bool(arguments.get("launch", False)),
            arguments.get("quick_play_singleplayer"),
            arguments.get("world_name"),
            bool(arguments.get("create_world", False)),
            arguments.get("world_gamemode", "creative"),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
            int(arguments.get("timeout_seconds", 120)),
            float(arguments.get("poll_interval_seconds", 2)),
            arguments.get("hotbar_slot", 0),
            arguments.get("give_item"),
            int(arguments.get("give_count", 1)),
            arguments.get("give_slot", 0),
            bool(arguments.get("give_select", True)),
            bool(arguments.get("give_replace", True)),
            bool(arguments.get("release_input", True)),
            bool(arguments.get("check_held_item", True)),
            bool(arguments.get("take_screenshot", True)),
            arguments.get("recipe_items"),
            int(arguments.get("recipe_width", 1)),
            int(arguments.get("recipe_height", 1)),
            arguments.get("expected_output"),
            arguments.get("recipe_id"),
            bool(arguments.get("require_inventory", False)),
            bool(arguments.get("craft_recipe", False)),
            bool(arguments.get("craft_give_inputs", True)),
            int(arguments.get("crafts", 1)),
            arguments.get("block", "minecraft:stone"),
            arguments.get("block_x"),
            arguments.get("block_y"),
            arguments.get("block_z"),
            bool(arguments.get("check_use_item", True)),
            bool(arguments.get("check_use_block", True)),
            bool(arguments.get("cleanup", True)),
            bool(arguments.get("include_logs", True)),
        )
    elif name == "mod_dev_loop":
        data = mod_dev_loop(
            ctx,
            arguments["project_dir"],
            arguments.get("profile_id"),
            bool(arguments.get("build", True)),
            arguments.get("build_command"),
            arguments.get("artifact_glob"),
            arguments.get("jar_path"),
            bool(arguments.get("import_mod", True)),
            arguments.get("import_file_name"),
            bool(arguments.get("replace_mod", True)),
            bool(arguments.get("install_agent", True)),
            bool(arguments.get("launch", True)),
            bool(arguments.get("keep_running", False)),
            arguments.get("quick_play_singleplayer"),
            arguments.get("world_name", "Power Mine Codex Dev Test"),
            bool(arguments.get("create_world", True)),
            arguments.get("world_gamemode", "creative"),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
            int(arguments.get("timeout_seconds", 120)),
            float(arguments.get("poll_interval_seconds", 2)),
            bool(arguments.get("use_sample_targets", True)),
            arguments.get("give_item"),
            int(arguments.get("give_count", 1)),
            arguments.get("give_slot", 0),
            arguments.get("recipe_items"),
            int(arguments.get("recipe_width", 1)),
            int(arguments.get("recipe_height", 1)),
            arguments.get("expected_output"),
            arguments.get("block"),
            bool(arguments.get("cleanup", True)),
            bool(arguments.get("include_logs", True)),
            bool(arguments.get("check_held_item", True)),
            bool(arguments.get("take_screenshot", True)),
            bool(arguments.get("check_use_item", True)),
            bool(arguments.get("check_use_block", True)),
            bool(arguments.get("craft_recipe", True)),
            bool(arguments.get("craft_give_inputs", True)),
            int(arguments.get("crafts", 1)),
        )
    elif name == "agent_world_open":
        data = agent_world_open(
            arguments["world"],
            bool(arguments.get("create", True)),
            arguments.get("gamemode", "creative"),
            arguments.get("seed"),
            arguments.get("display_name"),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
            int(arguments.get("timeout_seconds", 90)),
        )
    elif name == "agent_give_item":
        data = agent_http_request(
            "/inventory/give",
            method="POST",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            body={
                "item": arguments["item"],
                "count": int(arguments.get("count", 1)),
                "slot": int(arguments.get("slot", 0)),
                "select": bool(arguments.get("select", True)),
                "replace": bool(arguments.get("replace", True)),
            },
        )
    elif name == "agent_wait_ticks":
        data = agent_wait_ticks(
            int(arguments.get("ticks", 20)),
            int(arguments.get("timeout_seconds", 10)),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
        )
    elif name == "agent_use_item":
        data = agent_use_item(
            arguments.get("hand", "main"),
            arguments.get("slot"),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
        )
    elif name == "agent_use_block":
        data = agent_use_block(
            int(arguments["x"]),
            int(arguments["y"]),
            int(arguments["z"]),
            arguments.get("hand", "main"),
            arguments.get("side", "up"),
            arguments.get("slot"),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
        )
    elif name == "agent_interaction_check":
        data = agent_interaction_check(
            arguments["mode"],
            arguments.get("x"),
            arguments.get("y"),
            arguments.get("z"),
            arguments.get("block"),
            arguments.get("item"),
            int(arguments.get("count", 1)),
            arguments.get("slot", 0),
            arguments.get("hand", "main"),
            arguments.get("side", "up"),
            int(arguments.get("wait_ticks", 2)),
            int(arguments.get("timeout_seconds", 10)),
            bool(arguments.get("require_effect", False)),
            arguments.get("expect_used"),
            arguments.get("expect_screen_open"),
            arguments.get("expect_block_changed"),
            arguments.get("expect_stack_changed"),
            bool(arguments.get("cleanup", False)),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
        )
    elif name == "agent_health":
        data = agent_http_request("/health", port=int(arguments.get("port", DEFAULT_AGENT_PORT)), token=arguments.get("token"))
    elif name == "agent_capabilities":
        data = agent_capabilities(port=int(arguments.get("port", DEFAULT_AGENT_PORT)), token=arguments.get("token"))
    elif name == "agent_state":
        data = agent_http_request("/state", port=int(arguments.get("port", DEFAULT_AGENT_PORT)), token=arguments.get("token"))
    elif name == "agent_inventory":
        data = agent_http_request("/inventory", port=int(arguments.get("port", DEFAULT_AGENT_PORT)), token=arguments.get("token"))
    elif name == "agent_release_input":
        data = agent_http_request(
            "/input/release",
            method="POST",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
        )
    elif name == "agent_held_item_render":
        data = agent_http_request(
            "/render/held-item",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            query={"hand": arguments.get("hand", "main"), "delayMs": arguments.get("delay_ms")},
        )
    elif name == "agent_block_render":
        data = agent_http_request(
            "/render/block",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            query={
                "x": arguments.get("x"),
                "y": arguments.get("y"),
                "z": arguments.get("z"),
                "lookAt": arguments.get("look_at"),
                "delayMs": arguments.get("delay_ms"),
            },
        )
    elif name == "agent_screenshot":
        data = agent_http_request(
            "/render/screenshot",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
        )
    elif name == "agent_camera_look":
        screenshot = bool(arguments.get("screenshot", False))
        delay_ms = arguments.get("delay_ms")
        if delay_ms is None:
            delay_ms = 250 if screenshot else 0
        body = {
            "center": arguments.get("center", True) is not False,
            "screenshot": screenshot,
            "delayMs": int(delay_ms),
        }
        for key in ("yaw", "pitch", "x", "y", "z"):
            if arguments.get(key) is not None:
                body[key] = arguments.get(key)
        data = agent_http_request(
            "/camera/look",
            method="POST",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            body=body,
        )
    elif name == "agent_recipe_check":
        data = agent_http_request(
            "/recipe/check",
            method="POST",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            body={
                "width": int(arguments.get("width", 3)),
                "height": int(arguments.get("height", 3)),
                "items": arguments.get("items", []),
                "expectedOutput": arguments.get("expected_output", ""),
                "recipeId": arguments.get("recipe_id", ""),
                "requireInventory": bool(arguments.get("require_inventory", False)),
            },
        )
    elif name == "agent_craft_recipe":
        data = agent_craft_recipe(
            arguments.get("items", []),
            int(arguments.get("width", 3)),
            int(arguments.get("height", 3)),
            arguments.get("expected_output"),
            int(arguments.get("crafts", 1)),
            bool(arguments.get("require_inventory", True)),
            bool(arguments.get("consume", True)),
            bool(arguments.get("insert_output", True)),
            arguments.get("output_slot"),
            bool(arguments.get("replace_output", False)),
            int(arguments.get("port", DEFAULT_AGENT_PORT)),
            arguments.get("token"),
        )
    elif name == "agent_world_snapshot":
        data = agent_http_request(
            "/world/snapshot",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            query={
                "radius": arguments.get("radius", 2),
                "includeAir": str(bool(arguments.get("include_air", False))).lower(),
                "x": arguments.get("x"),
                "y": arguments.get("y"),
                "z": arguments.get("z"),
            },
        )
    elif name == "agent_select_hotbar_slot":
        data = agent_http_request(
            "/hotbar/select",
            method="POST",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            body={"slot": int(arguments["slot"])},
        )
    elif name == "agent_place_block":
        data = agent_http_request(
            "/block/place",
            method="POST",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            body={
                "x": int(arguments["x"]),
                "y": int(arguments["y"]),
                "z": int(arguments["z"]),
                "block": arguments.get("block", "minecraft:stone"),
            },
        )
    elif name == "agent_break_block":
        data = agent_http_request(
            "/block/break",
            method="POST",
            port=int(arguments.get("port", DEFAULT_AGENT_PORT)),
            token=arguments.get("token"),
            body={
                "x": int(arguments["x"]),
                "y": int(arguments["y"]),
                "z": int(arguments["z"]),
                "drop": bool(arguments.get("drop", False)),
            },
        )
    else:
        raise ValueError(f"unknown tool: {name}")
    return {"content": [{"type": "text", "text": json_dump(data, pretty=True)}]}


def run_mcp_server() -> int:
    while True:
        try:
            request = read_mcp_message(sys.stdin.buffer)
        except Exception as exc:
            log_mcp_exception(f"failed to read request: {exc}\n{traceback.format_exc(limit=5)}")
            write_mcp_message(mcp_error(None, -32700, f"parse error: {exc}"))
            continue
        if request is None:
            return 0
        method = request.get("method")
        request_id = request.get("id")
        if method is None:
            continue
        try:
            if method == "initialize":
                protocol = (request.get("params") or {}).get("protocolVersion", "2024-11-05")
                write_mcp_message(mcp_result(request_id, {
                    "protocolVersion": protocol,
                    "capabilities": {"tools": {"listChanged": False}},
                    "serverInfo": {"name": "power-mine", "version": "0.1.0"},
                }))
            elif method == "tools/list":
                write_mcp_message(mcp_result(request_id, {"tools": TOOLS}))
            elif method == "tools/call":
                params = request.get("params") or {}
                result = call_tool(params.get("name", ""), params.get("arguments") or {})
                write_mcp_message(mcp_result(request_id, result))
            elif method == "ping":
                write_mcp_message(mcp_result(request_id, {}))
            elif method.startswith("notifications/"):
                continue
            else:
                write_mcp_message(mcp_error(request_id, -32601, f"method not found: {method}"))
        except Exception as exc:
            details = f"{exc}\n{traceback.format_exc(limit=5)}"
            log_mcp_exception(details)
            if request_id is not None:
                write_mcp_message(mcp_result(request_id, {
                    "content": [{"type": "text", "text": details}],
                    "isError": True,
                }))
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Power Mine diagnostics for Codex")
    parser.add_argument("--data-dir", help="Power Mine data directory")
    parser.add_argument("--pretty", action="store_true", help="Pretty-print JSON output")
    subparsers = parser.add_subparsers(dest="command", required=True)

    subparsers.add_parser("list-profiles", help="List launcher profiles")

    diagnose_profile_parser = subparsers.add_parser("diagnose-profile", help="Diagnose a launcher profile")
    diagnose_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")
    diagnose_profile_parser.add_argument("--no-logs", action="store_true", help="Skip log analysis")
    diagnose_profile_parser.add_argument("--max-log-bytes", type=int, default=MAX_LOG_BYTES)
    diagnose_profile_parser.add_argument("--log-scope", choices=("recent", "latest_run"), default="recent")

    wait_profile_parser = subparsers.add_parser("wait-profile-ready", help="Wait for launched profile ready/crash logs")
    wait_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")
    wait_profile_parser.add_argument("--timeout-seconds", type=int, default=90)
    wait_profile_parser.add_argument("--poll-interval-seconds", type=float, default=2)
    wait_profile_parser.add_argument("--max-log-bytes", type=int, default=MAX_LOG_BYTES)

    create_profile_parser = subparsers.add_parser("create-profile", help="Create a launcher profile")
    create_profile_parser.add_argument("name")
    create_profile_parser.add_argument("--minecraft-version", default="1.20.1")
    create_profile_parser.add_argument("--loader", default="fabric")
    create_profile_parser.add_argument("--loader-version", default="latest")
    create_profile_parser.add_argument("--game-dir")
    create_profile_parser.add_argument("--min-memory", type=int, default=1024)
    create_profile_parser.add_argument("--max-memory", type=int, default=4096)
    create_profile_parser.add_argument("--install", action="store_true")

    install_java_parser = subparsers.add_parser("install-java", help="Install a managed Java runtime")
    install_java_parser.add_argument("version", nargs="?", type=int, default=21, help="Java major version")

    scaffold_parser = subparsers.add_parser("scaffold-fabric-mod", help="Create a minimal Fabric mod project")
    scaffold_parser.add_argument("project_dir")
    scaffold_parser.add_argument("mod_id")
    scaffold_parser.add_argument("--name")
    scaffold_parser.add_argument("--package")
    scaffold_parser.add_argument("--minecraft-version", default="1.20.1")
    scaffold_parser.add_argument("--loader-version", default="0.15.11")
    scaffold_parser.add_argument("--fabric-api-version")
    scaffold_parser.add_argument("--yarn-mappings")
    scaffold_parser.add_argument("--mod-version", default="0.1.0")
    scaffold_parser.add_argument("--replace", action="store_true")

    forge_scaffold_parser = subparsers.add_parser("scaffold-forge-1.7.10-mod", help="Create a minimal Forge 1.7.10 mod project")
    forge_scaffold_parser.add_argument("project_dir")
    forge_scaffold_parser.add_argument("mod_id")
    forge_scaffold_parser.add_argument("--name")
    forge_scaffold_parser.add_argument("--package")
    forge_scaffold_parser.add_argument("--forge-version", default="1.7.10-10.13.4.1614-1.7.10")
    forge_scaffold_parser.add_argument("--mod-version", default="0.1.0")
    forge_scaffold_parser.add_argument("--replace", action="store_true")

    build_project_parser = subparsers.add_parser("build-mod-project", help="Build a local mod project and return the newest jar")
    build_project_parser.add_argument("project_dir")
    build_project_parser.add_argument("--build-command")
    build_project_parser.add_argument("--artifact-glob")
    build_project_parser.add_argument("--timeout-seconds", type=int, default=300)

    diagnose_mod_parser = subparsers.add_parser("diagnose-mod", help="Diagnose a mod jar")
    diagnose_mod_parser.add_argument("jar_path")
    diagnose_mod_parser.add_argument("--profile-id", help="Compare against a launcher profile")

    diagnose_content_parser = subparsers.add_parser("diagnose-mod-content", help="Validate mod assets and recipes")
    diagnose_content_parser.add_argument("jar_path")

    import_mod_parser = subparsers.add_parser("import-mod", help="Import a mod jar into a profile")
    import_mod_parser.add_argument("jar_path")
    import_mod_parser.add_argument("--profile-id", help="Profile id; defaults to selected profile")
    import_mod_parser.add_argument("--file-name", help="Target file name in the profile mods folder")
    import_mod_parser.add_argument("--disabled", action="store_true", help="Install as .jar.disabled")
    import_mod_parser.add_argument("--replace", action="store_true", help="Replace an existing installed mod")

    set_mod_parser = subparsers.add_parser("set-mod-enabled", help="Enable or disable an installed mod")
    set_mod_parser.add_argument("file_name")
    set_mod_parser.add_argument("--profile-id", help="Profile id; defaults to selected profile")
    set_mod_parser.add_argument("--enabled", action=argparse.BooleanOptionalAction, default=True)

    delete_mod_parser = subparsers.add_parser("delete-mod", help="Delete an installed mod")
    delete_mod_parser.add_argument("file_name")
    delete_mod_parser.add_argument("--profile-id", help="Profile id; defaults to selected profile")

    build_agent_parser = subparsers.add_parser("build-agent", help="Build an in-game agent mod")
    build_agent_parser.add_argument("--target", default=FABRIC_AGENT_TARGET, help="fabric-1.20.1, forge-1.7.10, or forge-1.12.2")

    install_agent_parser = subparsers.add_parser("install-agent", help="Build and install the agent mod into a profile")
    install_agent_parser.add_argument("--profile-id", help="Profile id; defaults to selected profile")
    install_agent_parser.add_argument("--target", help="Explicit target: fabric-1.20.1, forge-1.7.10, or forge-1.12.2")
    install_agent_parser.add_argument("--no-build", action="store_true", help="Use the existing agent jar")
    install_agent_parser.add_argument("--no-replace", action="store_true", help="Do not replace an existing installed agent")

    install_profile_parser = subparsers.add_parser("install-profile", help="Install a profile via headless launcher")
    install_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")

    repair_profile_parser = subparsers.add_parser("repair-profile", help="Repair a profile via headless launcher")
    repair_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")

    launch_profile_parser = subparsers.add_parser("launch-profile", help="Launch a profile via headless launcher")
    launch_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")
    launch_profile_parser.add_argument("--quick-play-singleplayer", help="Existing singleplayer world name to open via Minecraft Quick Play")
    launch_profile_parser.add_argument("--keep-pause-on-lost-focus", action="store_true", help="Do not force pauseOnLostFocus:false before launch")

    agent_parent = argparse.ArgumentParser(add_help=False)
    agent_parent.add_argument("--port", type=int, default=DEFAULT_AGENT_PORT)
    agent_parent.add_argument("--token")

    smoke_parser = subparsers.add_parser("agent-smoke-test", parents=[agent_parent], help="Run one static+runtime agent smoke test")
    smoke_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")
    smoke_parser.add_argument("--jar-path", help="Optional mod jar to inspect before runtime checks")
    smoke_parser.add_argument("--launch", action="store_true", help="Launch the profile before waiting for the agent")
    smoke_parser.add_argument("--quick-play-singleplayer", help="Existing singleplayer world name to open via Minecraft Quick Play")
    smoke_parser.add_argument("--world-name", help="World name to open through the agent after launch/readiness")
    smoke_parser.add_argument("--create-world", action="store_true", help="Create the world through the agent when supported")
    smoke_parser.add_argument("--world-gamemode", default="creative", help="Game mode for an agent-created world")
    smoke_parser.add_argument("--timeout-seconds", type=int, default=120)
    smoke_parser.add_argument("--poll-interval-seconds", type=float, default=2)
    smoke_parser.add_argument("--hotbar-slot", type=int, default=0)
    smoke_parser.add_argument("--give-item", help="Item id to give before held-item render/visual check")
    smoke_parser.add_argument("--give-count", type=int, default=1)
    smoke_parser.add_argument("--give-slot", type=int, default=0)
    smoke_parser.add_argument("--no-give-select", action="store_true", help="Do not select the given hotbar slot")
    smoke_parser.add_argument("--no-give-replace", action="store_true", help="Fail if the target inventory slot is occupied")
    smoke_parser.add_argument("--no-release-input", action="store_true", help="Do not ask the agent to release mouse input after entering the world")
    smoke_parser.add_argument("--no-held-item", action="store_true", help="Skip held item render/visual check")
    smoke_parser.add_argument("--no-screenshot", action="store_true", help="Skip framebuffer screenshot")
    smoke_parser.add_argument(
        "--recipe-items",
        help="Comma-separated row-major item ids; omit for the default oak_log -> oak_planks check; empty value skips recipe check",
    )
    smoke_parser.add_argument("--recipe-width", type=int, default=1)
    smoke_parser.add_argument("--recipe-height", type=int, default=1)
    smoke_parser.add_argument("--expected-output", help="Expected recipe output; defaults to oak planks only for the default oak log check")
    smoke_parser.add_argument("--recipe-id")
    smoke_parser.add_argument("--require-inventory", action="store_true")
    smoke_parser.add_argument("--craft-recipe", action="store_true", help="Give inputs and perform a real inventory craft after recipe check")
    smoke_parser.add_argument("--no-craft-give-inputs", action="store_true", help="Do not pre-fill inventory before --craft-recipe")
    smoke_parser.add_argument("--crafts", type=int, default=1, help="Number of craft iterations for --craft-recipe")
    smoke_parser.add_argument("--block", default="minecraft:stone", help="Block id to place and render-check")
    smoke_parser.add_argument("--x", type=int, help="Optional block x; defaults beside the player")
    smoke_parser.add_argument("--y", type=int, help="Optional block y; defaults beside the player")
    smoke_parser.add_argument("--z", type=int, help="Optional block z; defaults beside the player")
    smoke_parser.add_argument("--no-use-item", action="store_true", help="Skip item right-click interaction check")
    smoke_parser.add_argument("--no-use-block", action="store_true", help="Skip block right-click interaction check")
    smoke_parser.add_argument("--no-cleanup", action="store_true", help="Leave the smoke-test block in the world")
    smoke_parser.add_argument("--no-logs", action="store_true", help="Skip latest-run log analysis after runtime checks")

    dev_loop_parser = subparsers.add_parser("mod-dev-loop", parents=[agent_parent], help="Build/import/launch/smoke-test a mod project")
    dev_loop_parser.add_argument("project_dir")
    dev_loop_parser.add_argument("--profile-id", help="Profile id; defaults to selected profile")
    dev_loop_parser.add_argument("--no-build", action="store_true", help="Use an existing artifact instead of building")
    dev_loop_parser.add_argument("--build-command", help="Optional build command, for example './gradlew build'")
    dev_loop_parser.add_argument("--artifact-glob", help="Project-relative artifact glob")
    dev_loop_parser.add_argument("--jar-path", help="Existing jar path to use")
    dev_loop_parser.add_argument("--no-import", action="store_true", help="Do not import the jar into the profile")
    dev_loop_parser.add_argument("--import-file-name", help="Target file name in the profile mods folder")
    dev_loop_parser.add_argument("--no-replace", action="store_true", help="Do not replace an existing installed mod")
    dev_loop_parser.add_argument("--no-install-agent", action="store_true", help="Do not build/install the Power Mine agent")
    dev_loop_parser.add_argument("--no-launch", action="store_true", help="Do not launch the profile before runtime checks")
    dev_loop_parser.add_argument("--keep-running", action="store_true", help="Keep Minecraft running after the smoke test")
    dev_loop_parser.add_argument("--quick-play-singleplayer", help="Existing singleplayer world name to open via Minecraft Quick Play")
    dev_loop_parser.add_argument("--world-name", default="Power Mine Codex Dev Test", help="World name to open/create through the agent")
    dev_loop_parser.add_argument("--create-world", dest="create_world", action="store_true", default=True, help="Create the world when missing")
    dev_loop_parser.add_argument("--no-create-world", dest="create_world", action="store_false", help="Only open an existing world")
    dev_loop_parser.add_argument("--world-gamemode", default="creative")
    dev_loop_parser.add_argument("--timeout-seconds", type=int, default=120)
    dev_loop_parser.add_argument("--poll-interval-seconds", type=float, default=2)
    dev_loop_parser.add_argument("--no-sample-targets", action="store_true", help="Do not infer sample_item/sample_block targets from the mod id")
    dev_loop_parser.add_argument("--give-item", help="Item id to give before held-item render/visual check")
    dev_loop_parser.add_argument("--give-count", type=int, default=1)
    dev_loop_parser.add_argument("--give-slot", type=int, default=0)
    dev_loop_parser.add_argument(
        "--recipe-items",
        help="Comma-separated row-major item ids; omit to infer scaffold defaults; empty value skips recipe check",
    )
    dev_loop_parser.add_argument("--recipe-width", type=int, default=1)
    dev_loop_parser.add_argument("--recipe-height", type=int, default=1)
    dev_loop_parser.add_argument("--expected-output")
    dev_loop_parser.add_argument("--no-craft-recipe", action="store_true", help="Skip real inventory craft after recipe check")
    dev_loop_parser.add_argument("--no-craft-give-inputs", action="store_true", help="Do not pre-fill inventory before the dev-loop craft")
    dev_loop_parser.add_argument("--crafts", type=int, default=1, help="Number of craft iterations in the dev-loop craft")
    dev_loop_parser.add_argument("--block", help="Block id to place and render-check")
    dev_loop_parser.add_argument("--no-cleanup", action="store_true", help="Leave the smoke-test block in the world")
    dev_loop_parser.add_argument("--no-logs", action="store_true", help="Skip latest-run log analysis")
    dev_loop_parser.add_argument("--no-held-item", action="store_true", help="Skip held item render/visual check")
    dev_loop_parser.add_argument("--no-screenshot", action="store_true", help="Skip framebuffer screenshot")
    dev_loop_parser.add_argument("--no-use-item", action="store_true", help="Skip item right-click interaction check")
    dev_loop_parser.add_argument("--no-use-block", action="store_true", help="Skip block right-click interaction check")

    subparsers.add_parser("agent-health", parents=[agent_parent], help="Check the running in-game agent")
    subparsers.add_parser("agent-capabilities", parents=[agent_parent], help="Read bridge capabilities from the running agent")
    subparsers.add_parser("agent-state", parents=[agent_parent], help="Read player/world state from the running agent")
    subparsers.add_parser("agent-inventory", parents=[agent_parent], help="Read inventory from the running agent")
    subparsers.add_parser("agent-release-input", parents=[agent_parent], help="Release captured mouse input and disable pause-on-lost-focus")

    world_open_parser = subparsers.add_parser("agent-world-open", parents=[agent_parent], help="Open or create a singleplayer world")
    world_open_parser.add_argument("world", help="World save name to open or create")
    world_open_parser.add_argument("--create", dest="create", action="store_true", default=True, help="Create the world when it is missing")
    world_open_parser.add_argument("--no-create", dest="create", action="store_false", help="Only open an existing world")
    world_open_parser.add_argument("--gamemode", default="creative")
    world_open_parser.add_argument("--seed", type=int)
    world_open_parser.add_argument("--display-name")
    world_open_parser.add_argument("--timeout-seconds", type=int, default=90)

    give_parser = subparsers.add_parser("agent-give-item", parents=[agent_parent], help="Put an item stack into an inventory slot")
    give_parser.add_argument("item", help="Item id to give")
    give_parser.add_argument("--count", type=int, default=1)
    give_parser.add_argument("--slot", type=int, default=0)
    give_parser.add_argument("--no-select", action="store_true", help="Do not select the given hotbar slot")
    give_parser.add_argument("--no-replace", action="store_true", help="Fail if the target inventory slot is occupied")

    wait_ticks_parser = subparsers.add_parser("agent-wait-ticks", parents=[agent_parent], help="Wait for Minecraft game ticks")
    wait_ticks_parser.add_argument("ticks", nargs="?", type=int, default=20)
    wait_ticks_parser.add_argument("--timeout-seconds", type=int, default=10)

    use_item_parser = subparsers.add_parser("agent-use-item", parents=[agent_parent], help="Use/right-click the selected held item")
    use_item_parser.add_argument("--hand", default="main", choices=("main", "offhand", "off"))
    use_item_parser.add_argument("--slot", type=int, help="Optional hotbar slot 0-8 to select before use")

    use_block_parser = subparsers.add_parser("agent-use-block", parents=[agent_parent], help="Use/right-click a block")
    use_block_parser.add_argument("x", type=int)
    use_block_parser.add_argument("y", type=int)
    use_block_parser.add_argument("z", type=int)
    use_block_parser.add_argument("--hand", default="main", choices=("main", "offhand", "off"))
    use_block_parser.add_argument("--side", default="up", choices=("up", "down", "north", "south", "west", "east"))
    use_block_parser.add_argument("--slot", type=int, help="Optional hotbar slot 0-8 to select before use")

    interaction_parser = subparsers.add_parser("agent-interaction-check", parents=[agent_parent], help="Run a structured item/block right-click check")
    interaction_parser.add_argument("mode", choices=("item", "block"))
    interaction_parser.add_argument("--x", type=int)
    interaction_parser.add_argument("--y", type=int)
    interaction_parser.add_argument("--z", type=int)
    interaction_parser.add_argument("--block", help="Optional block id to place before block interaction")
    interaction_parser.add_argument("--item", help="Optional item id to give/select before interaction")
    interaction_parser.add_argument("--count", type=int, default=1)
    interaction_parser.add_argument("--slot", type=int, default=0)
    interaction_parser.add_argument("--hand", default="main", choices=("main", "offhand", "off"))
    interaction_parser.add_argument("--side", default="up", choices=("up", "down", "north", "south", "west", "east"))
    interaction_parser.add_argument("--wait-ticks", type=int, default=2)
    interaction_parser.add_argument("--timeout-seconds", type=int, default=10)
    interaction_parser.add_argument("--require-effect", action="store_true")
    interaction_parser.add_argument("--expect-used", action=argparse.BooleanOptionalAction, default=None)
    interaction_parser.add_argument("--expect-screen-open", action=argparse.BooleanOptionalAction, default=None)
    interaction_parser.add_argument("--expect-block-changed", action=argparse.BooleanOptionalAction, default=None)
    interaction_parser.add_argument("--expect-stack-changed", action=argparse.BooleanOptionalAction, default=None)
    interaction_parser.add_argument("--cleanup", action="store_true", help="Restore/clear a block placed by this check")

    held_render_parser = subparsers.add_parser("agent-held-item-render", parents=[agent_parent], help="Check held item render model or visual probe")
    held_render_parser.add_argument("--hand", default="main", choices=("main", "offhand", "off"))
    held_render_parser.add_argument("--delay-ms", type=int, default=250)

    block_render_parser = subparsers.add_parser("agent-block-render", parents=[agent_parent], help="Check block render model or visual probe")
    block_render_parser.add_argument("--x", type=int)
    block_render_parser.add_argument("--y", type=int)
    block_render_parser.add_argument("--z", type=int)
    block_render_parser.add_argument("--no-look-at", action="store_true", help="Do not turn legacy-agent camera toward the block")
    block_render_parser.add_argument("--delay-ms", type=int, default=350)

    subparsers.add_parser("agent-screenshot", parents=[agent_parent], help="Capture a Minecraft screenshot through the agent")

    camera_parser = subparsers.add_parser("agent-camera-look", parents=[agent_parent], help="Rotate the client camera/head and optionally capture a screenshot")
    camera_parser.add_argument("--yaw", type=float, help="Absolute camera yaw in degrees; use with --pitch")
    camera_parser.add_argument("--pitch", type=float, help="Absolute camera pitch in degrees; use with --yaw")
    camera_parser.add_argument("--x", type=float, help="Target x coordinate; use with --y and --z")
    camera_parser.add_argument("--y", type=float, help="Target y coordinate; use with --x and --z")
    camera_parser.add_argument("--z", type=float, help="Target z coordinate; use with --x and --y")
    camera_parser.add_argument("--center", action=argparse.BooleanOptionalAction, default=True, help="Look at the center of the x/y/z block target")
    camera_parser.add_argument("--screenshot", action="store_true", help="Capture a framebuffer screenshot after rotating")
    camera_parser.add_argument("--delay-ms", type=int, help="Delay after movement; defaults to 250 with --screenshot, otherwise 0")

    recipe_parser = subparsers.add_parser("agent-recipe-check", parents=[agent_parent], help="Check a crafting grid through RecipeManager")
    recipe_parser.add_argument("items", help="Comma-separated row-major item ids; use empty entries or minecraft:air for blank slots")
    recipe_parser.add_argument("--width", type=int, default=3)
    recipe_parser.add_argument("--height", type=int, default=3)
    recipe_parser.add_argument("--expected-output")
    recipe_parser.add_argument("--recipe-id")
    recipe_parser.add_argument("--require-inventory", action="store_true")

    craft_parser = subparsers.add_parser("agent-craft-recipe", parents=[agent_parent], help="Craft a recipe through the running agent inventory")
    craft_parser.add_argument("items", help="Comma-separated row-major item ids; use empty entries or minecraft:air for blank slots")
    craft_parser.add_argument("--width", type=int, default=3)
    craft_parser.add_argument("--height", type=int, default=3)
    craft_parser.add_argument("--expected-output")
    craft_parser.add_argument("--crafts", type=int, default=1)
    craft_parser.add_argument("--no-require-inventory", action="store_true", help="Allow crafting even when inventory inputs are missing")
    craft_parser.add_argument("--no-consume", action="store_true", help="Do not consume recipe inputs")
    craft_parser.add_argument("--no-insert-output", action="store_true", help="Do not insert the crafted output stack")
    craft_parser.add_argument("--output-slot", type=int)
    craft_parser.add_argument("--replace-output", action="store_true", help="Replace output-slot when occupied")

    snapshot_parser = subparsers.add_parser("agent-world-snapshot", parents=[agent_parent], help="Read nearby blocks")
    snapshot_parser.add_argument("--radius", type=int, default=2)
    snapshot_parser.add_argument("--include-air", action="store_true")
    snapshot_parser.add_argument("--x", type=int)
    snapshot_parser.add_argument("--y", type=int)
    snapshot_parser.add_argument("--z", type=int)

    hotbar_parser = subparsers.add_parser("agent-select-hotbar-slot", parents=[agent_parent], help="Select hotbar slot 0-8")
    hotbar_parser.add_argument("slot", type=int)

    place_parser = subparsers.add_parser("agent-place-block", parents=[agent_parent], help="Place/set a block")
    place_parser.add_argument("x", type=int)
    place_parser.add_argument("y", type=int)
    place_parser.add_argument("z", type=int)
    place_parser.add_argument("--block", default="minecraft:stone")

    break_parser = subparsers.add_parser("agent-break-block", parents=[agent_parent], help="Break a block")
    break_parser.add_argument("x", type=int)
    break_parser.add_argument("y", type=int)
    break_parser.add_argument("z", type=int)
    break_parser.add_argument("--drop", action="store_true")

    subparsers.add_parser("mcp", help="Run the MCP stdio server")

    args = parser.parse_args(argv)
    if args.command == "mcp":
        return run_mcp_server()

    ctx = context_from_args(args.data_dir)
    try:
        if args.command == "list-profiles":
            output = list_profiles(ctx)
        elif args.command == "diagnose-profile":
            output = diagnose_profile(ctx, args.profile_id, not args.no_logs, args.max_log_bytes, args.log_scope)
        elif args.command == "wait-profile-ready":
            output = wait_profile_ready(ctx, args.profile_id, args.timeout_seconds, args.poll_interval_seconds, args.max_log_bytes)
        elif args.command == "create-profile":
            output = create_profile(
                ctx,
                args.name,
                args.minecraft_version,
                args.loader,
                args.loader_version,
                args.game_dir,
                args.min_memory,
                args.max_memory,
                args.install,
            )
        elif args.command == "install-java":
            output = install_java_runtime(ctx, args.version)
        elif args.command == "scaffold-fabric-mod":
            output = scaffold_fabric_mod(
                args.project_dir,
                args.mod_id,
                args.name,
                args.package,
                args.minecraft_version,
                args.loader_version,
                args.fabric_api_version,
                args.yarn_mappings,
                args.mod_version,
                args.replace,
            )
        elif args.command == "scaffold-forge-1.7.10-mod":
            output = scaffold_forge_1710_mod(
                args.project_dir,
                args.mod_id,
                args.name,
                args.package,
                args.forge_version,
                args.mod_version,
                args.replace,
            )
        elif args.command == "build-mod-project":
            output = build_mod_project(args.project_dir, args.build_command, args.artifact_glob, args.timeout_seconds)
        elif args.command == "diagnose-mod":
            profile = find_profile(ctx, args.profile_id) if args.profile_id else None
            output = inspect_mod(args.jar_path, profile)
        elif args.command == "diagnose-mod-content":
            output = diagnose_mod_content(args.jar_path)
        elif args.command == "import-mod":
            output = import_profile_mod(ctx, args.profile_id, args.jar_path, args.file_name, not args.disabled, args.replace)
        elif args.command == "set-mod-enabled":
            output = set_profile_mod_enabled(ctx, args.profile_id, args.file_name, args.enabled)
        elif args.command == "delete-mod":
            output = delete_profile_mod(ctx, args.profile_id, args.file_name)
        elif args.command == "build-agent":
            output = build_agent_mod(args.target)
        elif args.command == "install-agent":
            output = install_agent_mod(ctx, args.profile_id, not args.no_build, not args.no_replace, args.target)
        elif args.command == "install-profile":
            output = run_power_mine_headless(ctx, "install-profile", args.profile_id)
        elif args.command == "repair-profile":
            output = run_power_mine_headless(ctx, "repair-profile", args.profile_id)
        elif args.command == "launch-profile":
            output = launch_profile(ctx, args.profile_id, args.quick_play_singleplayer, args.keep_pause_on_lost_focus)
        elif args.command == "agent-smoke-test":
            if args.recipe_items is None:
                recipe_items = None
            elif args.recipe_items == "":
                recipe_items = []
            else:
                recipe_items = [item.strip() for item in args.recipe_items.split(",")]
            output = agent_smoke_test(
                ctx,
                args.profile_id,
                args.jar_path,
                args.launch,
                args.quick_play_singleplayer,
                args.world_name,
                args.create_world,
                args.world_gamemode,
                args.port,
                args.token,
                args.timeout_seconds,
                args.poll_interval_seconds,
                args.hotbar_slot,
                args.give_item,
                args.give_count,
                args.give_slot,
                not args.no_give_select,
                not args.no_give_replace,
                not args.no_release_input,
                not args.no_held_item,
                not args.no_screenshot,
                recipe_items,
                args.recipe_width,
                args.recipe_height,
                args.expected_output,
                args.recipe_id,
                args.require_inventory,
                args.craft_recipe,
                not args.no_craft_give_inputs,
                args.crafts,
                args.block,
                args.x,
                args.y,
                args.z,
                not args.no_use_item,
                not args.no_use_block,
                not args.no_cleanup,
                not args.no_logs,
            )
        elif args.command == "mod-dev-loop":
            if args.recipe_items is None:
                recipe_items = None
            elif args.recipe_items == "":
                recipe_items = []
            else:
                recipe_items = [item.strip() for item in args.recipe_items.split(",")]
            output = mod_dev_loop(
                ctx,
                args.project_dir,
                args.profile_id,
                not args.no_build,
                args.build_command,
                args.artifact_glob,
                args.jar_path,
                not args.no_import,
                args.import_file_name,
                not args.no_replace,
                not args.no_install_agent,
                not args.no_launch,
                args.keep_running,
                args.quick_play_singleplayer,
                args.world_name,
                args.create_world,
                args.world_gamemode,
                args.port,
                args.token,
                args.timeout_seconds,
                args.poll_interval_seconds,
                not args.no_sample_targets,
                args.give_item,
                args.give_count,
                args.give_slot,
                recipe_items,
                args.recipe_width,
                args.recipe_height,
                args.expected_output,
                args.block,
                not args.no_cleanup,
                not args.no_logs,
                not args.no_held_item,
                not args.no_screenshot,
                not args.no_use_item,
                not args.no_use_block,
                not args.no_craft_recipe,
                not args.no_craft_give_inputs,
                args.crafts,
            )
        elif args.command == "agent-health":
            output = agent_http_request("/health", port=args.port, token=args.token)
        elif args.command == "agent-capabilities":
            output = agent_capabilities(port=args.port, token=args.token)
        elif args.command == "agent-state":
            output = agent_http_request("/state", port=args.port, token=args.token)
        elif args.command == "agent-inventory":
            output = agent_http_request("/inventory", port=args.port, token=args.token)
        elif args.command == "agent-release-input":
            output = agent_http_request("/input/release", method="POST", port=args.port, token=args.token)
        elif args.command == "agent-world-open":
            output = agent_world_open(
                args.world,
                args.create,
                args.gamemode,
                args.seed,
                args.display_name,
                args.port,
                args.token,
                args.timeout_seconds,
            )
        elif args.command == "agent-give-item":
            output = agent_http_request(
                "/inventory/give",
                method="POST",
                port=args.port,
                token=args.token,
                body={
                    "item": args.item,
                    "count": args.count,
                    "slot": args.slot,
                    "select": not args.no_select,
                    "replace": not args.no_replace,
                },
            )
        elif args.command == "agent-wait-ticks":
            output = agent_wait_ticks(args.ticks, args.timeout_seconds, args.port, args.token)
        elif args.command == "agent-use-item":
            output = agent_use_item(args.hand, args.slot, args.port, args.token)
        elif args.command == "agent-use-block":
            output = agent_use_block(args.x, args.y, args.z, args.hand, args.side, args.slot, args.port, args.token)
        elif args.command == "agent-interaction-check":
            output = agent_interaction_check(
                args.mode,
                args.x,
                args.y,
                args.z,
                args.block,
                args.item,
                args.count,
                args.slot,
                args.hand,
                args.side,
                args.wait_ticks,
                args.timeout_seconds,
                args.require_effect,
                args.expect_used,
                args.expect_screen_open,
                args.expect_block_changed,
                args.expect_stack_changed,
                args.cleanup,
                args.port,
                args.token,
            )
        elif args.command == "agent-held-item-render":
            output = agent_http_request("/render/held-item", port=args.port, token=args.token, query={"hand": args.hand, "delayMs": args.delay_ms})
        elif args.command == "agent-block-render":
            output = agent_http_request(
                "/render/block",
                port=args.port,
                token=args.token,
                query={"x": args.x, "y": args.y, "z": args.z, "lookAt": not args.no_look_at, "delayMs": args.delay_ms},
            )
        elif args.command == "agent-screenshot":
            output = agent_http_request("/render/screenshot", port=args.port, token=args.token)
        elif args.command == "agent-camera-look":
            delay_ms = args.delay_ms
            if delay_ms is None:
                delay_ms = 250 if args.screenshot else 0
            body = {
                "center": args.center,
                "screenshot": args.screenshot,
                "delayMs": delay_ms,
            }
            for key in ("yaw", "pitch", "x", "y", "z"):
                value = getattr(args, key)
                if value is not None:
                    body[key] = value
            output = agent_http_request("/camera/look", method="POST", port=args.port, token=args.token, body=body)
        elif args.command == "agent-recipe-check":
            output = agent_http_request(
                "/recipe/check",
                method="POST",
                port=args.port,
                token=args.token,
                body={
                    "width": args.width,
                    "height": args.height,
                    "items": [item.strip() for item in args.items.split(",")],
                    "expectedOutput": args.expected_output or "",
                    "recipeId": args.recipe_id or "",
                    "requireInventory": args.require_inventory,
                },
            )
        elif args.command == "agent-craft-recipe":
            output = agent_craft_recipe(
                [item.strip() for item in args.items.split(",")],
                args.width,
                args.height,
                args.expected_output,
                args.crafts,
                not args.no_require_inventory,
                not args.no_consume,
                not args.no_insert_output,
                args.output_slot,
                args.replace_output,
                args.port,
                args.token,
            )
        elif args.command == "agent-world-snapshot":
            output = agent_http_request(
                "/world/snapshot",
                port=args.port,
                token=args.token,
                query={
                    "radius": args.radius,
                    "includeAir": str(args.include_air).lower(),
                    "x": args.x,
                    "y": args.y,
                    "z": args.z,
                },
            )
        elif args.command == "agent-select-hotbar-slot":
            output = agent_http_request("/hotbar/select", method="POST", port=args.port, token=args.token, body={"slot": args.slot})
        elif args.command == "agent-place-block":
            output = agent_http_request(
                "/block/place",
                method="POST",
                port=args.port,
                token=args.token,
                body={"x": args.x, "y": args.y, "z": args.z, "block": args.block},
            )
        elif args.command == "agent-break-block":
            output = agent_http_request(
                "/block/break",
                method="POST",
                port=args.port,
                token=args.token,
                body={"x": args.x, "y": args.y, "z": args.z, "drop": args.drop},
            )
        else:
            parser.error(f"unknown command: {args.command}")
    except Exception as exc:
        print(json_dump({"ok": False, "command": args.command, "error": str(exc)}, pretty=args.pretty))
        return 1
    print(json_dump(output, pretty=args.pretty))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
