#!/usr/bin/env python3
"""Build/test a narrowly scoped patch from the owner's exact KUAL JAR."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import urllib.request
from zipfile import ZipFile

ROOT = Path(__file__).resolve().parents[2]
SOURCES = Path(__file__).resolve().parent
ORIGINAL = "c13fc2bbb2890530a5c6374b879b2049a6136028713c37794d9f40093ea8dae3"
ASM_URL = "https://repo.maven.apache.org/maven2/org/ow2/asm/asm/9.7.1/asm-9.7.1.jar"
ASM_SHA256 = "8cadd43ac5eb6d09de05faecca38b917a040bb9139c7edeb4cc81c740b713281"
OWNER = "com/mobileread/ixtab/kindlelauncher/KualBooklet.class"
HELPER = "com/inkpika/kual/Navigation.class"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True, help="Original device KUALBooklet.jar")
    parser.add_argument("--java-home", type=Path, default=os.environ.get("JAVA_HOME"), help="JDK 9+ with javac/java")
    args = parser.parse_args()
    original = args.input.resolve()
    if digest(original) != ORIGINAL:
        parser.error("Unsupported or already modified KUAL JAR; no patch produced")
    javac = str(args.java_home / "bin/javac") if args.java_home else shutil.which("javac")
    java = str(args.java_home / "bin/java") if args.java_home else shutil.which("java")
    if not javac or not java:
        parser.error("Set --java-home to a host JDK with javac and java")
    cache = ROOT / "local/kual-navigation-build"
    cache.mkdir(parents=True, exist_ok=True)
    asm = cache / "asm-9.7.1.jar"
    if not asm.exists() or digest(asm) != ASM_SHA256:
        with urllib.request.urlopen(ASM_URL, timeout=30) as response:
            data = response.read(1024 * 1024)
        if hashlib.sha256(data).hexdigest() != ASM_SHA256:
            raise RuntimeError("ASM download checksum mismatch")
        asm.write_bytes(data)
    output = ROOT / "dist/kual-navigation"
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=cache) as temporary:
        work = Path(temporary)
        classes, tests = work / "classes", work / "tests"
        classes.mkdir(); tests.mkdir()
        cp = os.pathsep.join((str(original), str(asm)))
        subprocess.run([javac, "--release", "8", "-Xlint:-options", "-cp", cp, "-d", str(classes),
                        str(SOURCES / "Navigation.java"), str(SOURCES / "PatchKual.java")], check=True)
        patched = work / "KUALBooklet.jar"
        subprocess.run([java, "-cp", os.pathsep.join((str(classes), str(asm))), "PatchKual",
                        str(original), str(patched), str(classes / HELPER)], check=True)
        subprocess.run([javac, "--release", "8", "-Xlint:-options", "-cp", str(patched), "-d", str(tests),
                        str(SOURCES / "NavigationTest.java")], check=True)
        subprocess.run([java, "-Xverify:all", "-cp", os.pathsep.join((str(tests), str(patched))), "NavigationTest"], check=True)
        with ZipFile(original) as before, ZipFile(patched) as after:
            assert set(after.namelist()) - set(before.namelist()) == {HELPER}
            assert not set(before.namelist()) - set(after.namelist())
            for name in before.namelist():
                if name != OWNER:
                    assert before.read(name) == after.read(name), "Unexpected modified entry: " + name
            assert after.read(OWNER)[6:8] == b"\x00\x30", "KUAL bytecode version changed"
        # The input JAR is never modified or redistributed in the source tree.
        shutil.copyfile(patched, output / "KUALBooklet.jar")
        with ZipFile(output / "navigation-tests.jar", "w") as archive:
            for path in tests.rglob("*.class"):
                archive.write(path, path.relative_to(tests))
        shutil.copyfile(SOURCES / "device-install.sh", output / "device-install.sh")
        metadata = {"patch_version": "1.0.0", "original_sha256": ORIGINAL,
                    "patched_sha256": digest(patched), "kual": "v2.7.32-g74c69f9", "java_minimum": "8"}
        (output / "manifest.json").write_text(json.dumps(metadata, indent=2) + "\n")
        (output / "KUALBooklet.jar.sha256").write_text(metadata["patched_sha256"] + "  KUALBooklet.jar\n")
    print("Built and tested: " + str(output))


if __name__ == "__main__":
    main()
