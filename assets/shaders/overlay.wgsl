import package::byke::sprite;
import package::byke::globals;

// Full screen overlay above the world and the weapon: a dark vignette, film grain and a
// red vignette for damage. The sprite color carries the parameters (linear values):
// r = damage flash in [0, 1], g = low health in [0, 1].

@vertex
fn vs_main(in: sprite::VertexInput) -> sprite::VertexOutput {
    return sprite::default_sprite_vertex(in);
}

fn hash(p: vec2f) -> f32 {
    return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

// over blends a layer onto a premultiplied color
fn over(dst: vec4f, color: vec3f, alpha: f32) -> vec4f {
    return vec4(color * alpha + dst.rgb * (1.0 - alpha), alpha + dst.a * (1.0 - alpha));
}

@fragment
fn overlay(vo: sprite::VertexOutput) -> @location(0) vec4f {
    let size = globals::view.viewport.zw;
    let uv = vo.position.xy / size;
    let time = globals::globals.time;

    let damage = vo.color.r;
    let low_health = vo.color.g;

    // 0 in the center, 1 in the corners
    let aspect = vec2(size.x / size.y, 1.0);
    let r = length((uv - 0.5) * 2.0 * aspect) / length(aspect);

    var out = vec4(0.0);

    // film grain, new noise every frame
    let grain = hash(floor(vo.position.xy) + fract(time * 7.13) * vec2(311.7, 183.3)) - 0.5;
    out = over(out, vec3(step(0.0, grain)), abs(grain) * 0.05);

    // dark corners
    out = over(out, vec3(0.0), smoothstep(0.45, 1.1, r) * 0.6);

    // red from the edges: a flash when hit, a slow pulse at low health
    let pulse = low_health * (0.6 + 0.4 * sin(time * 4.0));
    let red = clamp(max(damage, pulse * 0.7), 0.0, 1.0);
    let reach = mix(0.95, 0.5, red);
    out = over(out, vec3(0.6, 0.0, 0.0), smoothstep(reach, reach + 0.5, r) * red * 0.7 + damage * 0.1);

    return vec4(out.rgb / max(out.a, 0.0001), out.a);
}
