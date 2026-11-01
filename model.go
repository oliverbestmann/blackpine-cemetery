package main

import (
	"github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
	"github.com/oliverbestmann/byke/byke2d/gltf"
)

// ModelPart is one node of a model.
type ModelPart struct {
	Name string

	// Parent is the index of the parent part, -1 for root parts.
	Parent int

	// Local is the transform relative to the parent part.
	Local Transform

	// Model is the transform relative to the model origin.
	Model glm.Mat4f

	// Mesh is nil for nodes without a mesh.
	Mesh *Mesh

	// Min and Max bound the mesh in the part's local space.
	Min, Max glm.Vec3f

	// data is the source of Mesh, for merging
	data meshBuilder
}

// Model is a glTF file converted once into shared meshes. Instances use the same meshes and an
// equal material, so they are drawn in batches. All Kenney models of a kit use one texture.
type Model struct {
	Parts    []ModelPart
	Material StandardMaterial

	// Merged is all meshes of the model in one, in model space.
	Merged *Mesh

	// Min and Max bound the whole model in model space.
	Min, Max glm.Vec3f

	// merged is the data of Merged
	merged meshBuilder

	// pieces caches Pieces, so the broken parts of all instances share their meshes
	pieces []*Model
}

// Pieces cuts the model into horizontal slabs, the parts it breaks into.
func (m *Model) Pieces() []*Model {
	if m.pieces != nil {
		return m.pieces
	}

	for _, slab := range m.merged.slice(modelPieces) {
		// slivers make no useful piece
		if len(slab.vertices) < 4 {
			continue
		}

		piece := &Model{Material: m.Material, Merged: slab.build(), merged: slab}
		piece.Min, piece.Max = piece.Merged.AABB()
		m.pieces = append(m.pieces, piece)
	}

	return m.pieces
}

const modelPieces = 4

// Size is the extent of the model's bounding box.
func (m *Model) Size() glm.Vec3f {
	return m.Max.Sub(m.Min)
}

// Part returns the part with the given name, or nil.
func (m *Model) Part(name string) *ModelPart {
	for idx := range m.Parts {
		if m.Parts[idx].Name == name {
			return &m.Parts[idx]
		}
	}

	return nil
}

func loadModel(h *gltf.Handle, material StandardMaterial) *Model {
	model := &Model{Material: material}

	var walk func(node gltf.Node, parent int, parentMatrix glm.Mat4f)
	walk = func(node gltf.Node, parent int, parentMatrix glm.Mat4f) {
		translation, scale, rotation := node.TransformComponents()
		local := Transform{Translation: translation, Scale: scale, Rotation: rotation}
		matrix := parentMatrix.Mul(local.Affine3())

		part := ModelPart{Name: node.Name, Parent: parent, Local: local, Model: matrix}
		if node.Mesh.IsSet {
			part.data = convertMesh(h, h.Meshes[node.Mesh.Get()])
			part.Mesh = part.data.build()
			part.Min, part.Max = part.Mesh.AABB()
		}

		model.Parts = append(model.Parts, part)
		index := len(model.Parts) - 1

		for _, child := range h.ChildNodes(node) {
			walk(child, index, matrix)
		}
	}

	for _, node := range h.Scene(0) {
		walk(node, -1, glm.IdentityMat4f())
	}

	model.merged = mergeParts(model.Parts)
	model.Merged = model.merged.build()
	model.Min, model.Max = model.Merged.AABB()

	return model
}

// convertMesh merges all primitives of a glTF mesh. Kenney meshes have one material.
func convertMesh(h *gltf.Handle, mesh gltf.Mesh) meshBuilder {
	var b meshBuilder

	for _, prim := range mesh.Primitives {
		vertices := h.Resolve(prim.MustGet("POSITION")).([]glm.Vec3f)
		normals := h.Resolve(prim.MustGet("NORMAL")).([]glm.Vec3f)

		uvs := make([]glm.Vec2f, len(vertices))
		if ref, ok := prim.Attributes["TEXCOORD_0"]; ok {
			uvs = h.Resolve(ref).([]glm.Vec2f)
		}

		var indices []uint32
		if prim.Indices.IsSet {
			indices = resolveIndices(h, prim.Indices.Get())
		} else {
			for idx := range vertices {
				indices = append(indices, uint32(idx))
			}
		}

		b.add(vertices, normals, uvs, indices, glm.IdentityMat4f())
	}

	return b
}

// resolveIndices reads an index accessor. byke's Resolve does not support 8 bit indices.
func resolveIndices(h *gltf.Handle, ref gltf.Ref) []uint32 {
	acc := &h.Accessors[ref]
	if acc.ComponentType != gltfUnsignedByte {
		return toUint32(h.Resolve(ref))
	}

	return toUint32(h.BytesForAccessor(acc, 1)[:acc.Count])
}

const gltfUnsignedByte = 5121

func toUint32(raw any) []uint32 {
	switch indices := raw.(type) {
	case []uint32:
		return indices

	case []uint16:
		out := make([]uint32, len(indices))
		for idx, value := range indices {
			out[idx] = uint32(value)
		}

		return out

	case []uint8:
		out := make([]uint32, len(indices))
		for idx, value := range indices {
			out[idx] = uint32(value)
		}

		return out
	}

	panic("unsupported index type")
}

// mergeParts merges the meshes of all parts into model space.
func mergeParts(parts []ModelPart) meshBuilder {
	var b meshBuilder

	for _, part := range parts {
		if part.Mesh == nil {
			continue
		}

		b.add(part.data.vertices, part.data.normals, part.data.uvs, part.data.indices, part.Model)
	}

	return b
}

// meshBuilder collects triangles with normals and uvs.
type meshBuilder struct {
	vertices []glm.Vec3f
	normals  []glm.Vec3f
	uvs      []glm.Vec2f
	indices  []uint32
}

func (b *meshBuilder) add(vertices, normals []glm.Vec3f, uvs []glm.Vec2f, indices []uint32, transform glm.Mat4f) {
	base := uint32(len(b.vertices))

	for idx, v := range vertices {
		b.vertices = append(b.vertices, transform.Transform3(v))

		// no shear or non uniform scale in the models, rotating the normal is enough
		n := transform.Transform3(normals[idx]).Sub(transform.Transform3(glm.Vec3f{}))
		b.normals = append(b.normals, n.Normalize())
		b.uvs = append(b.uvs, uvs[idx])
	}

	for _, idx := range indices {
		b.indices = append(b.indices, base+idx)
	}
}

func (b *meshBuilder) build() *Mesh {
	mesh := MeshOf(b.indices, b.vertices).
		WithAttributes(VertexAttributeNormal, b.normals).
		WithAttributes(VertexAttributeUV, b.uvs)

	mesh.ComputeTangents()
	return mesh
}

// SpawnParts spawns one entity per part below parent, and returns the entities in part order.
// Parts without a mesh get an entity too, they are the pivots of their children.
func (m *Model) SpawnParts(commands *byke.Commands, parent byke.EntityId, extra ...byke.ErasedComponent) []byke.EntityId {
	entities := make([]byke.EntityId, len(m.Parts))

	for idx, part := range m.Parts {
		parentId := parent
		if part.Parent >= 0 {
			parentId = entities[part.Parent]
		}

		components := []byke.ErasedComponent{byke.ChildOf{Parent: parentId}, part.Local, InheritVisibility}
		if part.Mesh != nil {
			components = append(components, MeshInstance{Mesh: part.Mesh}, m.Material)
		}

		entities[idx] = commands.Spawn(append(components, extra...)...).Id()
	}

	return entities
}
