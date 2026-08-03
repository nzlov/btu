package threemf

import (
	"path/filepath"
	"testing"
)

func TestAnalyzeMaterialUsageMeasuresTransformedMeshArea(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	writeTest3MF(t, sourcePath, map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF"},
		"filament_type":   []string{"PLA", "PLA"},
	}, map[string]string{
		mainModelName: `<model><metadata name="Application">BambuStudio</metadata><resources><object id="5"><components>` +
			`<component path="/3D/Objects/parts.model" objectid="1" transform="2 0 0 0 2 0 0 0 2 0 0 0"/>` +
			`<component path="/3D/Objects/parts.model" objectid="2" transform="1 0 0 0 1 0 0 0 1 0 0 0"/>` +
			`</components></object></resources></model>`,
		modelSettingsName: `<config><object id="5"><metadata key="extruder" value="1"/>` +
			`<part id="1"/><part id="2"><metadata key="extruder" value="2"/></part></object></config>`,
		"3D/Objects/parts.model": `<model><resources>` +
			`<object id="1"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>` +
			`<triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object>` +
			`<object id="2"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>` +
			`<triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object>` +
			`</resources></model>`,
	})

	source, err := openArchive(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.reader.Close()
	usage, err := analyzeMaterialUsage(source)
	if err != nil {
		t.Fatal(err)
	}
	if usage[1] <= usage[2]*3 {
		t.Fatalf("usage = %v, transformed T1 surface should be four times T2", usage)
	}
}

func TestAnalyzeMaterialUsageCountsPaintInsteadOfDefaultMaterial(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	writeTest3MF(t, sourcePath, map[string]any{
		"filament_colour": []string{"#000000", "#FFFFFF"},
		"filament_type":   []string{"PLA", "PLA"},
	}, map[string]string{
		mainModelName: `<model><metadata name="Application">BambuStudio</metadata><resources><object id="1"><mesh>` +
			`<vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>` +
			`<triangles><triangle v1="0" v2="1" v3="2" paint_color="8"/></triangles>` +
			`</mesh></object></resources></model>`,
		modelSettingsName: `<config><object id="1"><metadata key="extruder" value="1"/></object></config>`,
	})

	source, err := openArchive(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.reader.Close()
	usage, err := analyzeMaterialUsage(source)
	if err != nil {
		t.Fatal(err)
	}
	if usage[2] < 0.49 || usage[1] >= usage[2] {
		t.Fatalf("usage = %v, painted T2 should own the triangle surface", usage)
	}
}
