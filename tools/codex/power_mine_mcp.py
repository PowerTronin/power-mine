#!/usr/bin/env python3
"""Power Mine diagnostics CLI and MCP server.

The script intentionally uses only the Python standard library so Codex can run
it on a developer machine without installing a separate package first.
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import traceback
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

            if "META-INF/MANIFEST.MF" not in names:
                result["warnings"].append("jar has no META-INF/MANIFEST.MF")
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

    for metadata in result["metadata"]:
        result["issues"].extend(metadata.get("issues", []))
        result["warnings"].extend(metadata.get("warnings", []))

    if profile:
        result["compatibility"] = compatibility_with_profile(result, profile)
        for item in result["compatibility"]:
            if item.get("severity") == "error":
                result["issues"].append(item["message"])
            elif item.get("severity") == "warning":
                result["warnings"].append(item["message"])
    return result


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


def find_power_mine_binary() -> list[str]:
    configured = os.environ.get("POWER_MINE_BINARY")
    if configured:
        return [configured]
    repo = Path(os.environ.get("POWER_MINE_REPO", Path(__file__).resolve().parents[2])).expanduser()
    candidates = [
        repo / "build" / "bin" / "power-mine",
        repo / "dist" / "power-mine-0.1.0-linux-x86_64.appimage",
    ]
    for candidate in candidates:
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return [str(candidate)]
    if shutil.which("go") and (repo / "go.mod").is_file():
        return ["go", "run", "."]
    raise FileNotFoundError("Power Mine binary not found; set POWER_MINE_BINARY or build the launcher")


def run_power_mine_headless(ctx: DiagnosticContext, command: str, profile_id: str | None) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    repo = os.environ.get("POWER_MINE_REPO", str(Path(__file__).resolve().parents[2]))
    invocation = find_power_mine_binary() + [
        "headless",
        command,
        "--data-dir",
        str(ctx.data_dir),
        "--profile-id",
        profile["id"],
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


LOG_PATTERNS = [
    ("error", "unsupported_java", re.compile(r"UnsupportedClassVersionError|class file version", re.I), "Java version mismatch"),
    ("error", "missing_class", re.compile(r"NoClassDefFoundError|ClassNotFoundException", re.I), "Missing dependency or wrong loader/version"),
    ("error", "mixin_failed", re.compile(r"Mixin apply failed|MixinTransformerError|InjectionError|InvalidMixinException", re.I), "Mixin failure"),
    ("error", "missing_dependency", re.compile(r"requires.*(?:mod|dependency)|missing.*(?:mod|dependency)|ModResolutionException", re.I), "Missing dependency"),
    ("error", "duplicate_mod", re.compile(r"duplicate mod|DuplicateModsFoundException|Duplicate mod", re.I), "Duplicate mod"),
    ("error", "mod_loading", re.compile(r"ModLoadingException|Loading errors encountered|Failed to load mod", re.I), "Mod loading failure"),
    ("error", "exception", re.compile(r"\b(FATAL|ERROR)\b|Exception in thread|Caused by:", re.I), "Exception or error line"),
]


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


def analyze_log(path: Path, max_bytes: int) -> dict[str, Any]:
    content, truncated = read_log_tail(path, max_bytes)
    matches = []
    for index, line in enumerate(content.splitlines(), start=1):
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


def diagnose_profile(ctx: DiagnosticContext, profile_id: str | None = None, include_logs: bool = True, max_log_bytes: int = MAX_LOG_BYTES) -> dict[str, Any]:
    profile = find_profile(ctx, profile_id)
    mods = [inspect_mod(str(path), profile) for path in list_profile_mod_paths(profile)]
    logs = [analyze_log(path, max_log_bytes) for path in recent_log_paths(profile)] if include_logs else []
    issue_count = sum(len(mod.get("issues", [])) for mod in mods) + sum(len(log.get("matches", [])) for log in logs)
    warning_count = sum(len(mod.get("warnings", [])) for mod in mods)
    status = "error" if issue_count else "warning" if warning_count else "ok"
    return {
        "status": status,
        "profile": profile_brief(profile),
        "summary": {
            "mods": len(mods),
            "issues": issue_count,
            "warnings": warning_count,
            "logsAnalyzed": len(logs),
        },
        "mods": mods,
        "logs": logs,
    }


def list_profiles(ctx: DiagnosticContext) -> dict[str, Any]:
    data = load_profiles(ctx)
    return {
        "dataDir": data["dataDir"],
        "selectedProfileId": data["selectedProfileId"],
        "profiles": [profile_brief(profile) for profile in data.get("profiles", [])],
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
    sys.stdout.buffer.write(f"Content-Length: {len(raw)}\r\n\r\n".encode("ascii"))
    sys.stdout.buffer.write(raw)
    sys.stdout.buffer.flush()


def mcp_result(request_id: Any, result: Any) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def mcp_error(request_id: Any, code: int, message: str) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}


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
        )
    elif name == "diagnose_mod":
        profile = find_profile(ctx, arguments.get("profile_id")) if arguments.get("profile_id") else None
        data = inspect_mod(arguments["jar_path"], profile)
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
    elif name == "install_profile":
        data = run_power_mine_headless(ctx, "install-profile", arguments.get("profile_id"))
    elif name == "repair_profile":
        data = run_power_mine_headless(ctx, "repair-profile", arguments.get("profile_id"))
    elif name == "launch_profile":
        data = run_power_mine_headless(ctx, "launch-profile", arguments.get("profile_id"))
    else:
        raise ValueError(f"unknown tool: {name}")
    return {"content": [{"type": "text", "text": json_dump(data, pretty=True)}]}


def run_mcp_server() -> int:
    while True:
        request = read_mcp_message(sys.stdin.buffer)
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
                    "capabilities": {"tools": {}},
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

    diagnose_mod_parser = subparsers.add_parser("diagnose-mod", help="Diagnose a mod jar")
    diagnose_mod_parser.add_argument("jar_path")
    diagnose_mod_parser.add_argument("--profile-id", help="Compare against a launcher profile")

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

    install_profile_parser = subparsers.add_parser("install-profile", help="Install a profile via headless launcher")
    install_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")

    repair_profile_parser = subparsers.add_parser("repair-profile", help="Repair a profile via headless launcher")
    repair_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")

    launch_profile_parser = subparsers.add_parser("launch-profile", help="Launch a profile via headless launcher")
    launch_profile_parser.add_argument("profile_id", nargs="?", help="Profile id; defaults to selected profile")

    subparsers.add_parser("mcp", help="Run the MCP stdio server")

    args = parser.parse_args(argv)
    if args.command == "mcp":
        return run_mcp_server()

    ctx = context_from_args(args.data_dir)
    try:
        if args.command == "list-profiles":
            output = list_profiles(ctx)
        elif args.command == "diagnose-profile":
            output = diagnose_profile(ctx, args.profile_id, not args.no_logs, args.max_log_bytes)
        elif args.command == "diagnose-mod":
            profile = find_profile(ctx, args.profile_id) if args.profile_id else None
            output = inspect_mod(args.jar_path, profile)
        elif args.command == "import-mod":
            output = import_profile_mod(ctx, args.profile_id, args.jar_path, args.file_name, not args.disabled, args.replace)
        elif args.command == "set-mod-enabled":
            output = set_profile_mod_enabled(ctx, args.profile_id, args.file_name, args.enabled)
        elif args.command == "delete-mod":
            output = delete_profile_mod(ctx, args.profile_id, args.file_name)
        elif args.command == "install-profile":
            output = run_power_mine_headless(ctx, "install-profile", args.profile_id)
        elif args.command == "repair-profile":
            output = run_power_mine_headless(ctx, "repair-profile", args.profile_id)
        elif args.command == "launch-profile":
            output = run_power_mine_headless(ctx, "launch-profile", args.profile_id)
        else:
            parser.error(f"unknown command: {args.command}")
    except Exception as exc:
        print(json_dump({"ok": False, "command": args.command, "error": str(exc)}, pretty=args.pretty))
        return 1
    print(json_dump(output, pretty=args.pretty))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
