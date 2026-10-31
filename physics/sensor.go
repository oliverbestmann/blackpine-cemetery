package physics

import (
	"slices"

	b3 "github.com/oliverbestmann/box3d-go"
	"github.com/oliverbestmann/byke"
)

// SensorOverlaps lists the dynamic bodies overlapping a Sensor body, updated every fixed step.
type SensorOverlaps struct {
	byke.Component[SensorOverlaps]
	Entities []byke.EntityId

	// Mass is the total mass of the overlapping bodies in kg.
	Mass float32

	// Frozen stops the updates, the sensor keeps its last values.
	Frozen bool
}

func sensorOverlapsSystem(
	sensors byke.Query[struct {
		_        byke.With[Sensor]
		Body     physicsBody
		Overlaps *SensorOverlaps
	}],
) {
	for sensor := range sensors.Items() {
		if sensor.Overlaps.Frozen {
			continue
		}

		var entities []byke.EntityId
		var mass float32

		var seen []b3.BodyId
		for _, shape := range sensor.Body.Body.GetShapes(nil) {
			for _, visitor := range shape.GetSensorData(nil) {
				body := visitor.GetBody()

				// only loose objects, kinematic scene parts such as the rocket also enter sensors.
				// a body with many shapes counts once
				if body.GetType() != b3.DynamicBody || slices.Contains(seen, body.Id) {
					continue
				}

				seen = append(seen, body.Id)
				entities = append(entities, byke.EntityId(body.GetUserData()))
				mass += body.GetMass()
			}
		}

		sensor.Overlaps.Entities = entities
		sensor.Overlaps.Mass = mass
	}
}
