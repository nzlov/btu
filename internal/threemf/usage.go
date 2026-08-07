package threemf

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	pathpkg "path"
	"strconv"
	"strings"
)

type materialUsage map[int]float64

func (usage materialUsage) add(material int, weight float64) {
	if material <= 0 {
		return
	}
	if weight <= 0 {
		if _, exists := usage[material]; !exists {
			usage[material] = 0
		}
		return
	}
	usage[material] += weight
}

type plateMaterialUsage struct {
	ID        int
	Name      string
	Materials materialUsage
}

type projectMaterialUsage struct {
	Total  materialUsage
	Plates []plateMaterialUsage
}

func (usage *projectMaterialUsage) add(material int, weight float64, plateIndexes []int) {
	if len(plateIndexes) == 0 {
		usage.Total.add(material, weight)
		return
	}
	for _, plateIndex := range plateIndexes {
		usage.Total.add(material, weight)
		usage.Plates[plateIndex].Materials.add(material, weight)
	}
}

type settingsMetadataXML struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}

type settingsPartXML struct {
	ID       int                   `xml:"id,attr"`
	Metadata []settingsMetadataXML `xml:"metadata"`
}

type settingsObjectXML struct {
	ID       int                   `xml:"id,attr"`
	Metadata []settingsMetadataXML `xml:"metadata"`
	Parts    []settingsPartXML     `xml:"part"`
}

type settingsConfigXML struct {
	Objects []settingsObjectXML `xml:"object"`
	Plates  []settingsPlateXML  `xml:"plate"`
}

type settingsPlateXML struct {
	Metadata  []settingsMetadataXML      `xml:"metadata"`
	Instances []settingsModelInstanceXML `xml:"model_instance"`
}

type settingsModelInstanceXML struct {
	Metadata []settingsMetadataXML `xml:"metadata"`
}

type componentXML struct {
	Path      string `xml:"path,attr"`
	ObjectID  int    `xml:"objectid,attr"`
	Transform string `xml:"transform,attr"`
}

type resourceObjectXML struct {
	ID         int            `xml:"id,attr"`
	Mesh       *struct{}      `xml:"mesh"`
	Components []componentXML `xml:"components>component"`
}

type packageModelXML struct {
	Objects []resourceObjectXML `xml:"resources>object"`
}

type meshKey struct {
	path     string
	objectID int
}

type meshAssignment struct {
	material     int
	transform    transform3D
	plateIndexes []int
}

type transform3D [12]float64

func identityTransform() transform3D {
	return transform3D{1, 0, 0, 0, 1, 0, 0, 0, 1}
}

func parseTransform(value string) (transform3D, error) {
	if strings.TrimSpace(value) == "" {
		return identityTransform(), nil
	}
	parts := strings.Fields(value)
	if len(parts) != 12 {
		return transform3D{}, fmt.Errorf("invalid 3MF transform %q", value)
	}
	var result transform3D
	for index, part := range parts {
		number, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return transform3D{}, fmt.Errorf("invalid 3MF transform number %q", part)
		}
		result[index] = number
	}
	return result, nil
}

type point3D struct {
	x float64
	y float64
	z float64
}

func (transform transform3D) apply(point point3D) point3D {
	return point3D{
		x: point.x*transform[0] + point.y*transform[3] + point.z*transform[6] + transform[9],
		y: point.x*transform[1] + point.y*transform[4] + point.z*transform[7] + transform[10],
		z: point.x*transform[2] + point.y*transform[5] + point.z*transform[8] + transform[11],
	}
}

func materialUsageWorkTotal(source archive) int {
	total := 2
	for _, file := range source.reader.File {
		if strings.HasSuffix(file.Name, ".model") {
			total++
		}
	}
	return total
}

func analyzeMaterialUsage(source archive, progress itemProgressFunc) (projectMaterialUsage, error) {
	total := materialUsageWorkTotal(source)
	current := 0
	modelSettings, err := readMember(source.files[modelSettingsName])
	if err != nil {
		return projectMaterialUsage{}, fmt.Errorf("read model settings: %w", err)
	}
	current++
	reportItemProgress(progress, current, total, modelSettingsName)
	mainModel, err := readMember(source.files[mainModelName])
	if err != nil {
		return projectMaterialUsage{}, fmt.Errorf("read main model: %w", err)
	}
	current++
	reportItemProgress(progress, current, total, mainModelName)

	usage := projectMaterialUsage{Total: make(materialUsage)}
	assignments, err := modelAssignments(modelSettings, mainModel, &usage)
	if err != nil {
		return projectMaterialUsage{}, err
	}
	for _, file := range source.reader.File {
		name := file.Name
		if !strings.HasSuffix(name, ".model") {
			continue
		}
		if !hasAssignmentsForPath(assignments, name) {
			current++
			reportItemProgress(progress, current, total, name)
			continue
		}
		data, err := readMember(file)
		if err != nil {
			return projectMaterialUsage{}, fmt.Errorf("read %s: %w", name, err)
		}
		if err := measureModelUsage(data, name, assignments, &usage); err != nil {
			return projectMaterialUsage{}, fmt.Errorf("analyze %s: %w", name, err)
		}
		current++
		reportItemProgress(progress, current, total, name)
	}
	return usage, nil
}

func modelAssignments(modelSettings, mainModel []byte, usage *projectMaterialUsage) (map[meshKey][]meshAssignment, error) {
	var settings settingsConfigXML
	if err := xml.Unmarshal(modelSettings, &settings); err != nil {
		return nil, fmt.Errorf("parse model settings: %w", err)
	}
	var model packageModelXML
	if err := xml.Unmarshal(mainModel, &model); err != nil {
		return nil, fmt.Errorf("parse main model: %w", err)
	}
	resources := make(map[int]resourceObjectXML, len(model.Objects))
	for _, object := range model.Objects {
		resources[object.ID] = object
	}
	plateIndexes := buildPlateUsage(settings, usage)

	assignments := make(map[meshKey][]meshAssignment)
	for _, object := range settings.Objects {
		defaultMaterial := metadataMaterial(object.Metadata)
		if defaultMaterial == 0 {
			defaultMaterial = 1
		}
		objectPlates, printable := plateIndexes[object.ID]
		usage.add(defaultMaterial, 0, objectPlates)
		resource, found := resources[object.ID]
		if !found {
			for _, part := range object.Parts {
				usage.add(inheritedMaterial(part.Metadata, defaultMaterial), 0, objectPlates)
			}
			continue
		}
		if !printable {
			continue
		}
		if len(resource.Components) == 0 {
			assignments[meshKey{path: mainModelName, objectID: object.ID}] = append(
				assignments[meshKey{path: mainModelName, objectID: object.ID}],
				meshAssignment{material: defaultMaterial, transform: identityTransform(), plateIndexes: objectPlates},
			)
			continue
		}

		parts := make(map[int]settingsPartXML, len(object.Parts))
		for _, part := range object.Parts {
			parts[part.ID] = part
		}
		for _, component := range resource.Components {
			// Bambu part IDs match component object IDs in the referenced .model package member.
			material := defaultMaterial
			if part, ok := parts[component.ObjectID]; ok {
				material = inheritedMaterial(part.Metadata, defaultMaterial)
			}
			usage.add(material, 0, objectPlates)
			transform, err := parseTransform(component.Transform)
			if err != nil {
				return nil, fmt.Errorf("object %d component %d: %w", object.ID, component.ObjectID, err)
			}
			key := meshKey{path: normalizeModelPath(component.Path), objectID: component.ObjectID}
			assignments[key] = append(assignments[key], meshAssignment{
				material: material, transform: transform, plateIndexes: objectPlates,
			})
		}
	}
	return assignments, nil
}

func buildPlateUsage(settings settingsConfigXML, usage *projectMaterialUsage) map[int][]int {
	result := make(map[int][]int)
	if len(settings.Plates) == 0 {
		usage.Plates = []plateMaterialUsage{{ID: 1, Materials: make(materialUsage)}}
		for _, object := range settings.Objects {
			result[object.ID] = []int{0}
		}
		return result
	}

	usage.Plates = make([]plateMaterialUsage, len(settings.Plates))
	for index, plate := range settings.Plates {
		plateID, _ := strconv.Atoi(metadataValue(plate.Metadata, "plater_id"))
		if plateID <= 0 {
			plateID = index + 1
		}
		name := metadataValue(plate.Metadata, "plater_name")
		for _, instance := range plate.Instances {
			objectID, _ := strconv.Atoi(metadataValue(instance.Metadata, "object_id"))
			if objectID <= 0 {
				continue
			}
			result[objectID] = append(result[objectID], index)
		}
		usage.Plates[index] = plateMaterialUsage{ID: plateID, Name: name, Materials: make(materialUsage)}
	}
	return result
}

func metadataValue(metadata []settingsMetadataXML, key string) string {
	for _, item := range metadata {
		if item.Key == key {
			return item.Value
		}
	}
	return ""
}

func metadataMaterial(metadata []settingsMetadataXML) int {
	for _, item := range metadata {
		if item.Key != "extruder" {
			continue
		}
		material, _ := strconv.Atoi(item.Value)
		return material
	}
	return 0
}

func inheritedMaterial(metadata []settingsMetadataXML, fallback int) int {
	if material := metadataMaterial(metadata); material > 0 {
		return material
	}
	return fallback
}

func normalizeModelPath(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "/")
	return pathpkg.Clean(value)
}

func hasAssignmentsForPath(assignments map[meshKey][]meshAssignment, name string) bool {
	for key := range assignments {
		if key.path == name {
			return true
		}
	}
	return false
}

func measureModelUsage(data []byte, name string, assignments map[meshKey][]meshAssignment, usage *projectMaterialUsage) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	objectID := 0
	var vertices []point3D
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "object":
				objectID, err = intAttribute(token.Attr, "id")
				if err != nil {
					return err
				}
				vertices = nil
			case "vertex":
				point, err := vertexAttributes(token.Attr)
				if err != nil {
					return err
				}
				vertices = append(vertices, point)
			case "triangle":
				meshAssignments := assignments[meshKey{path: name, objectID: objectID}]
				if len(meshAssignments) == 0 {
					continue
				}
				indices, paint, err := triangleAttributes(token.Attr)
				if err != nil {
					return err
				}
				if indices[0] >= len(vertices) || indices[1] >= len(vertices) || indices[2] >= len(vertices) {
					return fmt.Errorf("object %d triangle references an unknown vertex", objectID)
				}
				states := []int{0}
				if paint != "" {
					states, err = paintStates(paint)
					if err != nil {
						return err
					}
				}
				for _, assignment := range meshAssignments {
					area := triangleArea(
						assignment.transform.apply(vertices[indices[0]]),
						assignment.transform.apply(vertices[indices[1]]),
						assignment.transform.apply(vertices[indices[2]]),
					)
					share := area / float64(len(states))
					for _, state := range states {
						if state == 0 {
							state = assignment.material
						}
						usage.add(state, share, assignment.plateIndexes)
					}
				}
			}
		case xml.EndElement:
			if token.Name.Local == "object" {
				objectID = 0
				vertices = nil
			}
		}
	}
}

func intAttribute(attributes []xml.Attr, name string) (int, error) {
	for _, attribute := range attributes {
		if attribute.Name.Local != name {
			continue
		}
		value, err := strconv.Atoi(attribute.Value)
		if err != nil || value < 0 {
			return 0, fmt.Errorf("invalid %s attribute %q", name, attribute.Value)
		}
		return value, nil
	}
	return 0, fmt.Errorf("missing %s attribute", name)
}

func floatAttribute(attributes []xml.Attr, name string) (float64, error) {
	for _, attribute := range attributes {
		if attribute.Name.Local != name {
			continue
		}
		value, err := strconv.ParseFloat(attribute.Value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid %s attribute %q", name, attribute.Value)
		}
		return value, nil
	}
	return 0, fmt.Errorf("missing %s attribute", name)
}

func vertexAttributes(attributes []xml.Attr) (point3D, error) {
	x, err := floatAttribute(attributes, "x")
	if err != nil {
		return point3D{}, err
	}
	y, err := floatAttribute(attributes, "y")
	if err != nil {
		return point3D{}, err
	}
	z, err := floatAttribute(attributes, "z")
	if err != nil {
		return point3D{}, err
	}
	return point3D{x: x, y: y, z: z}, nil
}

func triangleAttributes(attributes []xml.Attr) ([3]int, string, error) {
	var result [3]int
	for index, name := range []string{"v1", "v2", "v3"} {
		value, err := intAttribute(attributes, name)
		if err != nil {
			return result, "", err
		}
		result[index] = value
	}
	paint := ""
	for _, attribute := range attributes {
		if attribute.Name.Local == "paint_color" {
			paint = attribute.Value
			break
		}
	}
	return result, paint, nil
}

func triangleArea(a, b, c point3D) float64 {
	ab := point3D{x: b.x - a.x, y: b.y - a.y, z: b.z - a.z}
	ac := point3D{x: c.x - a.x, y: c.y - a.y, z: c.z - a.z}
	cross := point3D{
		x: ab.y*ac.z - ab.z*ac.y,
		y: ab.z*ac.x - ab.x*ac.z,
		z: ab.x*ac.y - ab.y*ac.x,
	}
	return math.Sqrt(cross.x*cross.x+cross.y*cross.y+cross.z*cross.z) / 2
}
