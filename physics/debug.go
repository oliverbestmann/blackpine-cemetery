package physics

import (
	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// PluginDebugColliders renders the colliders of dynamic bodies and sensors as transparent meshes.
//
// shortcut: only hull shapes (boxes, cylinders, hulls) are drawn, add spheres, capsules and
// meshes when they are used on dynamic bodies or sensors.
func PluginDebugColliders(app *byke.App) {
	app.AddSystems(byke.Update, func(commands *byke.Commands, bodies byke.Query[debugBody]) {
		debugColliders(commands, bodies, false)
	})
}

// PluginDebugSensors works like PluginDebugColliders, but only renders the sensors.
func PluginDebugSensors(app *byke.App) {
	app.AddSystems(byke.Update, func(commands *byke.Commands, bodies byke.Query[debugBody]) {
		debugColliders(commands, bodies, true)
	})
}

type debugBody struct {
	_ byke.Added[physicsBody]

	EntityId  byke.EntityId
	RigidBody RigidBody
	Body      physicsBody
	Sensor    byke.Has[Sensor]
}

func debugColliders(commands *byke.Commands, bodies byke.Query[debugBody], sensorsOnly bool) {
	for item := range bodies.Items() {
		if !item.Sensor.Exists() && (sensorsOnly || item.RigidBody.Type != Dynamic) {
			continue
		}

		// emissive, so the colliders stand out in dark places like the inside of the rocket
		color := byke2d.ColorSRGBA(0.2, 0.8, 1.0, 0.4)
		emission := glm.Vec3f{0.03, 0.6, 1}
		if item.Sensor.Exists() {
			color = byke2d.ColorSRGBA(1.0, 0.3, 0.8, 0.5)
			emission = glm.Vec3f{1, 0.07, 0.6}
		}

		for _, shape := range item.Body.Body.GetShapes(nil) {
			if shape.GetType() != b3.HullShape {
				continue
			}

			// the hull is in the unscaled body frame, the child inherits the entity's scale, undo it
			transform := byke2d.NewTransform()
			transform.Scale = glm.Vec3f{1, 1, 1}.Div(item.Body.scale)

			commands.Spawn(
				byke.ChildOf{Parent: item.EntityId},
				transform,
				byke2d.MeshInstance{Mesh: hullMesh(shape.GetHull())},
				byke2d.StandardMaterial{
					BaseColor:           color,
					EmissiveScale:       emission,
					PerceptualRoughness: 1,
					AlphaMode:           byke2d.AlphaModeBlend,
				},
				// sensors are often hidden, show the collider anyway
				byke2d.Visible,
			)
		}
	}
}

// hullMesh triangulates the faces of a hull, with separate vertices per face for flat shading.
func hullMesh(hull *b3.HullData) *byke2d.Mesh {
	points, edges := hull.Points(), hull.Edges()

	var vertices []glm.Vec3f
	var indices []uint32

	planes := hull.Planes()

	for faceIdx, face := range hull.Faces() {
		first := uint32(len(vertices))

		// walk the half edges around the face
		edge := face.Edge
		for {
			vertices = append(vertices, toGlmVec(points[edges[edge].Origin]))
			edge = edges[edge].Next
			if edge == face.Edge {
				break
			}
		}

		// fan triangulation, faces are convex. keep the triangles counter clockwise seen from outside
		outward := toGlmVec(planes[faceIdx].Normal)
		polygon := vertices[first:]
		flip := len(polygon) >= 3 && polygon[1].Sub(polygon[0]).Cross(polygon[2].Sub(polygon[0])).Dot(outward) < 0

		for idx := first + 1; idx+1 < uint32(len(vertices)); idx++ {
			if flip {
				indices = append(indices, first, idx+1, idx)
			} else {
				indices = append(indices, first, idx, idx+1)
			}
		}
	}

	// the standard material needs uvs, the debug color has no texture
	mesh := byke2d.MeshOf(indices, vertices).WithAttributes(byke2d.VertexAttributeUV, make([]glm.Vec2f, len(vertices)))
	mesh.ComputeNormals()
	return mesh
}
