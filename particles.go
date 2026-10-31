package main

import (
	. "github.com/oliverbestmann/byke"
	. "github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// Particle moves a short lived visual effect: sparks, smoke, fire, tracers.
type Particle struct {
	Component[Particle]

	Velocity glm.Vec3f

	// Gravity in m/s², positive pulls down.
	Gravity float32

	// Drag slows the particle down, per second.
	Drag float32

	Age, Life float32

	// the scale goes from StartScale to EndScale over the lifetime
	StartScale, EndScale glm.Vec3f

	// Bounce makes it stop on the ground instead of falling through.
	Bounce bool
}

// maxParticles caps the particles, older ones die early when there are more.
const maxParticles = 1500

func particlesPlugin(app *App) {
	app.AddSystems(Update, particleSystem)
}

func particleSystem(vt VirtualTime, commands *Commands, particles Query[struct {
	EntityId  EntityId
	Particle  *Particle
	Transform *Transform
}],
) {
	dt := vt.DeltaSecs
	speedUp := max(1, float32(particles.Count())/maxParticles)

	for item := range particles.Items() {
		p := item.Particle
		p.Age += dt * speedUp

		if p.Age >= p.Life {
			commands.Entity(item.EntityId).Despawn()
			continue
		}

		p.Velocity[1] -= p.Gravity * dt
		p.Velocity = p.Velocity.Scale(decay(p.Drag, dt))

		pos := item.Transform.Translation.Add(p.Velocity.Scale(dt))
		if p.Bounce && pos[1] < 0.05 {
			pos[1] = 0.05
			p.Velocity = glm.Vec3f{p.Velocity[0] * 0.5, -p.Velocity[1] * 0.3, p.Velocity[2] * 0.5}
		}

		item.Transform.Translation = pos

		t := p.Age / p.Life
		item.Transform.Scale = p.StartScale.Add(p.EndScale.Sub(p.StartScale).Scale(t))
	}
}

// spawnParticle spawns a particle with the given mesh and material.
func spawnParticle(commands *Commands, mesh *Mesh, material StandardMaterial, position glm.Vec3f, particle Particle) {
	commands.Spawn(
		Round{},
		TransformFromXYZ(position.XYZ()).WithScaleXYZ(particle.StartScale.XYZ()),
		MeshInstance{Mesh: mesh},
		material,
		NotShadowCaster{},
		particle,
	)
}
