import package::byke::sprite;
import package::byke::globals;

// A procedural campfire flame on a camera facing sprite. The texture is not used.
// The sprite color alpha carries a per fire time offset, so fires do not flicker in sync.

@vertex
fn vs_main(in: sprite::VertexInput) -> sprite::VertexOutput {
    return sprite::default_sprite_vertex(in);
}

fn hash(p: vec2f) -> f32 {
    return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

// smooth value noise between 0 and 1
fn noise(p: vec2f) -> f32 {
    let i = floor(p);
    let f = fract(p);
    let u = f * f * (3.0 - 2.0 * f);

    let a = hash(i);
    let b = hash(i + vec2(1.0, 0.0));
    let c = hash(i + vec2(0.0, 1.0));
    let d = hash(i + vec2(1.0, 1.0));

    return mix(mix(a, b, u.x), mix(c, d, u.x), u.y);
}

// sums up noise of increasing frequency for more detail
fn fbm(p: vec2f) -> f32 {
    var value = 0.0;
    var amplitude = 0.5;
    var q = p;

    for (var i = 0; i < 5; i++) {
        value += amplitude * noise(q);
        q *= 2.0;
        amplitude *= 0.5;
    }

    return value;
}

@fragment
fn fire(vo: sprite::VertexOutput) -> @location(0) vec4f {
    let time = globals::globals.time + vo.color.a * 100.0;

    // 0 at the bottom, 1 at the top
    let height = 1.0 - vo.uv.y;

    // distance from the vertical center line
    let dx = abs(vo.uv.x - 0.5) * 2.0;

    // noise that moves upwards, a bit of sideways sway
    let sway = (noise(vec2(time * 0.7, 3.1)) - 0.5) * 0.25 * height;
    let n = fbm(vec2((vo.uv.x + sway) * 4.0, vo.uv.y * 3.0 + time * 2.2));

    // the flame gets thinner towards the top and the sides
    let v = clamp((n - height * 0.85 - dx * (0.6 + height)) * 3.5, 0.0, 1.0);

    // saturated red to orange, bright enough for bloom
    let color = vec3(4.0 * v, 1.6 * v * v, 0.3 * v * v * v * v);

    // fade in at the bottom edge
    let alpha = clamp(v * 2.0, 0.0, 1.0) * smoothstep(0.0, 0.08, height);

    // premultiplied
    return vec4(color * alpha, alpha);
}
