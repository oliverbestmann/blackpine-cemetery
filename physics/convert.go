package physics

import (
	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

func toB3Vec(v glm.Vec3f) b3.Vec3 {
	return b3.Vec3{X: v.X, Y: v.Y, Z: v.Z}
}

func toGlmVec(v b3.Vec3) glm.Vec3f {
	return glm.V3f(v.X, v.Y, v.Z)
}

func toB3Quat(q glm.Quat) b3.Quat {
	v := q.Values()
	return b3.Quat{V: b3.Vec3{X: v.X, Y: v.Y, Z: v.Z}, S: v.W}
}

func toGlmQuat(q b3.Quat) glm.Quat {
	return glm.QuatOf(q.V.X, q.V.Y, q.V.Z, q.S)
}

// decompose splits an affine matrix into translation, rotation and scale.
// Shear and negative scale are not supported.
func decompose(m glm.Mat4f) (translation glm.Vec3f, rotation glm.Quat, scale glm.Vec3f) {
	var rot glm.Mat3f
	var lengths [3]float32
	for col := range 3 {
		axis := glm.V3f(m[col][0], m[col][1], m[col][2])
		lengths[col] = axis.Length()
		rot[col] = axis.Scale(1 / lengths[col]).Array()
	}

	return m.Translation(), glm.QuatFromMat3(rot), glm.V3f(lengths[0], lengths[1], lengths[2])
}

// poseMatrix builds the matrix of a rigid transform, without scale.
func poseMatrix(t b3.Transform) glm.Mat4f {
	mat := glm.TranslationMat4f(t.P.X, t.P.Y, t.P.Z)
	mat.RotateAssign(toGlmQuat(t.Q))
	return mat
}
