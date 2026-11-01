"""Verify browser SBOM completeness, artifact identity and dependency references."""
import hashlib
import unittest
from unittest.mock import patch

from sbom import ROOT, VENDOR, add_browser_components, add_runtime_license, browser_components


class SBOMTest(unittest.TestCase):
    def test_browser_inventory_covers_every_bundled_asset_and_license(self):
        components = browser_components()
        files = [f for c in components for f in c["components"]]
        expected = {p.relative_to(ROOT).as_posix() for p in VENDOR.rglob("*")
                    if p.is_file() and p.name != "manifest.json"}
        self.assertEqual(expected, {f["name"] for f in files})
        for file in files:
            self.assertEqual(hashlib.sha256((ROOT / file["name"]).read_bytes()).hexdigest(), file["hashes"][0]["content"])
        for component in components:
            self.assertEqual("MIT", component["licenses"][0]["license"]["id"])
            self.assertIn("Permission is hereby granted", component["licenses"][0]["license"]["text"]["content"])

    def test_merge_preserves_go_dependencies_and_connects_browser_graph(self):
        bom = {"metadata": {"component": {"bom-ref": "teleport"}},
               "components": [{"bom-ref": "go-module"}],
               "dependencies": [{"ref": "teleport", "dependsOn": ["go-module"]}]}
        add_browser_components(bom)
        refs = {"teleport", *(c["bom-ref"] for c in bom["components"])}
        refs.update(f["bom-ref"] for c in bom["components"] for f in c.get("components", []))
        self.assertEqual(len(refs), len(bom["components"]) + 1 + sum(len(c.get("components", [])) for c in bom["components"]))
        self.assertIn("go-module", bom["dependencies"][0]["dependsOn"])
        self.assertEqual(4, len(bom["dependencies"][0]["dependsOn"]))
        for dependency in bom["dependencies"]:
            self.assertIn(dependency["ref"], refs)
            self.assertTrue(set(dependency["dependsOn"]).issubset(refs))

    def test_integrity_failure_prevents_sbom_generation(self):
        with patch("sbom.verify_web", side_effect=AssertionError("changed vendor artifact")):
            with self.assertRaisesRegex(AssertionError, "changed vendor artifact"):
                browser_components()

    def test_standard_library_retains_the_active_toolchain_license(self):
        bom = {"components": [{"name": "std"}, {"name": "other"}]}
        add_runtime_license(bom)
        license = bom["components"][0]["licenses"][0]["license"]
        self.assertEqual("BSD-3-Clause", license["id"])
        self.assertIn("The Go Authors", license["text"]["content"])
        self.assertNotIn("licenses", bom["components"][1])


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
