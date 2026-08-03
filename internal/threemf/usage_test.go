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
	if usage.Total[1] <= usage.Total[2]*3 {
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
	if usage.Total[2] < 0.49 || usage.Total[1] >= usage.Total[2] {
		t.Fatalf("usage = %v, painted T2 should own the triangle surface", usage)
	}
}

func TestAnalyzeMaterialUsageSeparatesNamedPlates(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.3mf")
	triangle := `<mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>` +
		`<triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh>`
	writeTest3MF(t, sourcePath, map[string]any{
		"filament_colour": []string{"#FFFFFF", "#000000"},
		"filament_type":   []string{"PLA", "PLA"},
	}, map[string]string{
		mainModelName: `<model><metadata name="Application">BambuStudio</metadata><resources>` +
			`<object id="1">` + triangle + `</object><object id="2">` + triangle + `</object></resources></model>`,
		modelSettingsName: `<config>` +
			`<object id="1"><metadata key="extruder" value="1"/></object>` +
			`<object id="2"><metadata key="extruder" value="2"/></object>` +
			`<plate><metadata key="plater_id" value="3"/><metadata key="plater_name" value="eyes"/>` +
			`<model_instance><metadata key="object_id" value="1"/></model_instance></plate>` +
			`<plate><metadata key="plater_id" value="7"/><metadata key="plater_name" value="body"/>` +
			`<model_instance><metadata key="object_id" value="2"/></model_instance></plate></config>`,
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
	if len(usage.Plates) != 2 || usage.Plates[0].ID != 3 || usage.Plates[0].Name != "eyes" ||
		usage.Plates[1].ID != 7 || usage.Plates[1].Name != "body" {
		t.Fatalf("plates = %+v", usage.Plates)
	}
	if usage.Plates[0].Materials[1] <= 0 || usage.Plates[0].Materials[2] != 0 ||
		usage.Plates[1].Materials[2] <= 0 || usage.Plates[1].Materials[1] != 0 {
		t.Fatalf("plate usage = %+v", usage.Plates)
	}
}
