package physics

import (
	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// PluginDebugColliders renders the colliders of all bodies and sensors as transparent meshes.
// Bodies with NoDebugCollider are skipped.
//
// shortcut: only hulls (boxes, cylinders, hulls) and triangle meshes are drawn, add spheres
// and capsules when they need checking.
func PluginDebugColliders(app *byke.App) {
	app.AddSystems(byke.Update, func(commands *byke.Commands, bodies byke.Query[debugBody]) {
		debugColliders(commands, bodies, false)
	})
}

// NoDebugCollider hides the collider of a body from PluginDebugColliders, e.g. for the ground.
type NoDebugCollider struct {
	byke.ImmutableComponent[NoDebugCollider]
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
	Hidden    byke.Has[NoDebugCollider]
}

func debugColliders(commands *byke.Commands, bodies byke.Query[debugBody], sensorsOnly bool) {
	for item := range bodies.Items() {
		if item.Hidden.Exists() || (sensorsOnly && !item.Sensor.Exists()) {
			continue
		}

		// emissive, so the colliders stand out in dark places like the inside of the rocket
		color := byke2d.ColorSRGBA(0.2, 0.8, 1.0, 0.4)
		emission := glm.V3f(0.03, 0.6, 1)
		if item.Sensor.Exists() {
			color = byke2d.ColorSRGBA(1.0, 0.3, 0.8, 0.5)
			emission = glm.V3f(1, 0.07, 0.6)
		}

		for _, shape := range item.Body.Body.GetShapes(nil) {
			var mesh *byke2d.Mesh
			switch shape.GetType() {
			case b3.HullShape:
				mesh = hullMesh(shape.GetHull())
			case b3.MeshShape:
				mesh = triangleMesh(shape.GetMesh())
			default:
				continue
			}

			// the hull is in the unscaled body frame, the child inherits the entity's scale, undo it
			transform := byke2d.NewTransform()
			transform.Scale = glm.V3f(1, 1, 1).Div(item.Body.scale)

			commands.Spawn(
				byke.ChildOf{Parent: item.EntityId},
				transform,
				byke2d.MeshInstance{Mesh: mesh},
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

// triangleMesh copies the triangles of a mesh shape, with separate vertices per triangle for flat shading.
func triangleMesh(shape b3.Mesh) *byke2d.Mesh {
	points := shape.Data.Vertices()
	scale := toGlmVec(shape.Scale)

	var vertices []glm.Vec3f
	for _, tri := range shape.Data.Triangles() {
		for _, idx := range []int32{tri.Index1, tri.Index2, tri.Index3} {
			vertices = append(vertices, toGlmVec(points[idx]).Mul(scale))
		}
	}

	indices := make([]uint32, len(vertices))
	for idx := range indices {
		indices[idx] = uint32(idx)
	}

	mesh := byke2d.MeshOf(indices, vertices).WithAttributes(byke2d.VertexAttributeUV, make([]glm.Vec2f, len(vertices)))
	mesh.ComputeNormals()
	return mesh
}
