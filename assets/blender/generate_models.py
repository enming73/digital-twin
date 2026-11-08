"""Create original conceptual SMT assets. Run with Blender 4.2 in background mode.

    blender -b --factory-startup --python assets/blender/generate_models.py

Coordinates below are in scene space (x, vertical y, depth z), metres.
No third-party reference geometry, branding, or external textures are used.
"""
from pathlib import Path
import bpy

ROOT = Path(__file__).resolve().parents[2]


def material(name, color, metallic=0.0, roughness=0.55):
    mat = bpy.data.materials.new(name)
    mat.diffuse_color = (*color, 1)
    mat.use_nodes = True
    shader = mat.node_tree.nodes.get('Principled BSDF')
    shader.inputs['Base Color'].default_value = (*color, 1)
    shader.inputs['Metallic'].default_value = metallic
    shader.inputs['Roughness'].default_value = roughness
    return mat


def group(name):
    obj = bpy.data.objects.new(name, None)
    bpy.context.scene.collection.objects.link(obj)
    return obj


def box(name, position, size, mat, parent, bevel=0.015):
    x, y, z = position
    sx, sy, sz = size
    bpy.ops.mesh.primitive_cube_add(size=1, location=(x, -z, y))
    obj = bpy.context.object
    obj.name = name
    obj.dimensions = (sx, sz, sy)
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    if bevel:
        mod = obj.modifiers.new('Machined edges', 'BEVEL')
        mod.width = bevel
        mod.segments = 1
        bpy.ops.object.modifier_apply(modifier=mod.name)
    obj.data.materials.append(mat)
    obj.parent = parent
    return obj


def build(kind):
    bpy.ops.object.select_all(action='SELECT')
    bpy.ops.object.delete(use_global=False)
    shell = material('Powder coated warm grey', (0.68, 0.72, 0.69), 0.27, 0.48)
    trim = material('Dark graphite', (0.13, 0.17, 0.16), 0.38, 0.5)
    glass = material('Smoked protective window', (0.10, 0.18, 0.17), 0.25, 0.28)
    metal = material('Satin aluminium', (0.48, 0.55, 0.51), 0.75, 0.37)
    accent = material('Safety amber', (0.65, 0.35, 0.10), 0.2, 0.48)
    cabinet = group('Cabinet')
    conveyor = group('Conveyor')
    tower = group('Signal_Tower')
    for name in ['PCB_Anchor', 'Gantry_X', 'Gantry_Y', 'Placement_Head', 'Feeders']:
        group(name)
    box('Base plinth', (0, .14, 0), (2.6, .28, 1.92), trim, cabinet)
    box('Lower cabinet', (0, .57, 0), (2.52, .85, 1.82), shell, cabinet)
    for x in [-.61, .61]:
        box('Service door', (x, .56, .925), (1.12, .65, .03), shell, cabinet, .01)
        box('Door latch', (x + .40, .59, .95), (.045, .16, .025), trim, cabinet, .003)
    box('Upper housing', (0, 1.48, 0), (2.48, .82, 1.76), shell, cabinet)
    box('Front window frame', (0, 1.55, .90), (1.84, .49, .05), trim, cabinet)
    box('Front safety glazing', (0, 1.55, .936), (1.70, .37, .025), glass, cabinet, .005)
    box('Control bezel', (.98, 1.56, .93), (.27, .34, .055), trim, cabinet)
    box('Control display', (.98, 1.60, .965), (.19, .16, .02), glass, cabinet, .003)
    box('Upper aluminium sill', (0, 1.18, .93), (2.32, .07, .08), metal, cabinet)
    for x in [-1.07, 1.07]:
        box('Corner column', (x, 1.55, .91), (.10, .67, .09), metal, cabinet)
    if kind == 'mounter':
        box('Feeder rack', (0, 1.25, 1.00), (1.92, .12, .24), trim, cabinet)
        for i in range(12):
            box('Feeder slot', (-.78 + .14 * i, 1.32, 1.03), (.08, .06, .16), metal, cabinet, .003)
    else:
        box('Printer worktable', (0, 1.25, 1.0), (1.65, .09, .17), metal, cabinet)
        for x in [-.7, .7]:
            box('Clamp detail', (x, 1.34, 1.0), (.10, .10, .12), trim, cabinet, .004)
    box('Conveyor bed', (0, 1.01, 0), (3.13, .12, .72), trim, conveyor)
    for z in [-.43, .43]:
        box('Rail', (0, 1.12, z), (3.13, .07, .07), metal, conveyor)
    box('Tower mast', (1.01, 1.94, -.65), (.07, .38, .07), metal, tower, .005)
    box('Tower amber', (1.01, 2.17, -.65), (.12, .16, .12), accent, tower, .005)
    bpy.context.scene['provenance'] = 'Original generic conceptual SMT machine; no proprietary geometry.'
    source = ROOT / 'assets' / 'blender' / (kind + '.blend')
    output = ROOT / 'public' / 'models' / (kind + '.glb')
    output.parent.mkdir(parents=True, exist_ok=True)
    bpy.ops.wm.save_as_mainfile(filepath=str(source), compress=False)
    bpy.ops.export_scene.gltf(filepath=str(output), export_format='GLB', export_yup=True, export_apply=True)


for model in ['printer', 'mounter']:
    build(model)
