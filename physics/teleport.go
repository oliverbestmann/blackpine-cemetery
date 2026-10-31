package physics

import (
	"github.com/oliverbestmann/byke"
	"github.com/oliverbestmann/byke/byke2d"
	"github.com/oliverbestmann/byke/byke2d/glm"
)

// Disabled takes the body out of the simulation while the component is present.
type Disabled struct {
	byke.ImmutableComponent[Disabled]
}

// Teleport moves the body to Position in the next fixed step and stops it.
// The component removes itself afterwards.
type Teleport struct {
	byke.ImmutableComponent[Teleport]
	Position glm.Vec3f

	// Rotation replaces the rotation of the body if Rotate is set.
	Rotation glm.Quat
	Rotate   bool
}

func teleportSystem(commands *byke.Commands, bodies byke.Query[struct {
	EntityId       byke.EntityId
	Teleport       Teleport
	Body           physicsBody
	Transform      *byke2d.Transform
	LinearVelocity *LinearVelocity
}],
) {
	for item := range bodies.Items() {
		body := item.Body.Body
		rotation := body.GetRotation()
		if item.Teleport.Rotate {
			rotation = toB3Quat(item.Teleport.Rotation)
			item.Transform.Rotation = item.Teleport.Rotation
		}

		body.SetTransform(toB3Vec(item.Teleport.Position), rotation)
		body.SetLinearVelocity(toB3Vec(glm.Vec3f{}))
		body.SetAngularVelocity(toB3Vec(glm.Vec3f{}))
		item.Transform.Translation = item.Teleport.Position
		item.LinearVelocity.Vec3f = glm.Vec3f{}

		commands.Entity(item.EntityId).Remove[Teleport]()
	}
}

func disableSystem(bodies byke.Query[struct {
	Body     physicsBody
	Disabled byke.Has[Disabled]
}],
) {
	for item := range bodies.Items() {
		enabled := item.Body.Body.IsEnabled() != 0
		if enabled == item.Disabled.Exists() {
			if enabled {
				item.Body.Body.Disable()
			} else {
				item.Body.Body.Enable()
			}
		}
	}
}
