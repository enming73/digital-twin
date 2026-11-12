"""Create original conceptual SMT assets. Run with Blender 4.2+ in background mode.

    blender -b --factory-startup --python assets/blender/generate_models.py

Coordinates below are in scene space (x, vertical y, depth z), metres.
No third-party reference geometry, branding, or external textures are used.
Anchor node names are a contract with src/data/anchors.ts; keep them stable.
"""
from pathlib import Path
import bpy

ROOT = Path(__file__).resolve().parents[2]
KINDS = ['loader', 'printer', 'spi', 'mounter', 'reflow', 'aoi', 'unloader']


def material(name, color, metallic=0.0, roughness=0.55):
    mat = bpy.data.materials.new(name)
    mat.diffuse_color = (*color, 1)
    mat.use_nodes = True
    shader = mat.node_tree.nodes.get('Principled BSDF')
    shader.inputs['Base Color'].default_value = (*color, 1)
    shader.inputs['Metallic'].default_value = metallic
    shader.inputs['Roughness'].default_value = roughness
    return mat


def group(name, parent=None):
    obj = bpy.data.objects.new(name, None)
    bpy.context.scene.collection.objects.link(obj)
    obj.parent = parent
    return obj


def box(name, position, size, mat, parent, bevel=0.012):
    """Axis-aligned bevelled block in scene space; Blender is Z-up, scene is Y-up."""
    x, y, z = position
    sx, sy, sz = size
    bpy.ops.mesh.primitive_cube_add(size=1, location=(x, -z, y))
    obj = bpy.context.object
    obj.name = name
    obj.dimensions = (sx, sz, sy)
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    if bevel:
        mod = obj.modifiers.new('Machined edges', 'BEVEL')
        mod.width = min(bevel, min(sx, sy, sz) * 0.25)
        mod.segments = 2
        bpy.ops.object.modifier_apply(modifier=mod.name)
    obj.data.materials.append(mat)
    obj.parent = parent
    return obj


def reset():
    bpy.ops.object.select_all(action='SELECT')
    bpy.ops.object.delete(use_global=False)


def shell(mats):
    """Shared cabinet, conveyor and signal tower: the same visual language for every station."""
    shell_mat, trim, glass, metal, amber, plaque = mats
    cabinet = group('Cabinet')
    conveyor = group('Conveyor')
    tower = group('Signal_Tower')
    lens = group('Signal_Lens', tower)

    box('Base plinth', (0, .14, 0), (2.62, .28, 1.94), trim, cabinet, .008)
    box('Lower cabinet', (0, .57, 0), (2.52, .85, 1.82), shell_mat, cabinet)
    for side, x in (('L', -.61), ('R', .61)):
        box(f'Service door {side}', (x, .56, .925), (1.10, .66, .035), shell_mat, cabinet, .006)
        box(f'Door hinge {side}', (x * 1.78, .56, .95), (.05, .10, .02), trim, cabinet, .002)
        box(f'Door latch {side}', (x * .38, .60, .95), (.05, .16, .025), trim, cabinet, .003)
    box('Upper housing', (0, 1.48, 0), (2.48, .82, 1.76), shell_mat, cabinet)
    box('Front window frame', (0, 1.55, .90), (1.84, .49, .05), trim, cabinet)
    box('Front safety glazing', (0, 1.55, .936), (1.70, .37, .022), glass, cabinet, .004)
    box('Control bezel', (.98, 1.56, .93), (.27, .34, .055), trim, cabinet)
    box('Control display', (.98, 1.60, .965), (.19, .16, .018), glass, cabinet, .003)
    box('Upper aluminium sill', (0, 1.18, .93), (2.32, .07, .08), metal, cabinet)
    for x in (-1.07, 1.07):
        box('Corner column', (x, 1.55, .91), (.10, .67, .09), metal, cabinet)
    for x in (-.55, .55):
        box('Roof vent', (x, 1.892, -.28), (.62, .006, .10), trim, cabinet, .001)
    box('Nameplate', (0, .30, .945), (.58, .11, .018), plaque, cabinet, .002)
    box('Warning plaque', (-.86, 1.52, .958), (.17, .17, .012), plaque, cabinet, .002)

    box('Conveyor bed', (0, 1.01, 0), (3.13, .12, .72), trim, conveyor)
    box('Conveyor belt', (0, 1.075, 0), (3.05, .012, .52), trim, conveyor, .001)
    for z in (-.43, .43):
        box('Conveyor rail', (0, 1.12, z), (3.13, .07, .07), metal, conveyor)

    box('Tower mast', (1.01, 1.94, -.65), (.07, .38, .07), metal, tower, .004)
    box('Tower lens', (1.01, 2.17, -.65), (.12, .16, .12), amber, lens, .004)
    box('Tower cap', (1.01, 2.26, -.65), (.14, .03, .14), trim, tower, .002)

    group('PCB_Anchor').location = (0, -.12, 1.08)


def printer_parts(mats):
    _, trim, _, metal, _, _ = mats
    cabinet = group('Cabinet')
    box('Printer worktable', (0, 1.25, .95), (1.55, .09, .12), metal, cabinet)
    stencil = group('Stencil')
    box('Stencil frame', (0, 1.43, .95), (1.45, .02, .36), trim, stencil, .004)
    box('Stencil sheet', (0, 1.445, .95), (1.30, .006, .30), metal, stencil, .001)
    squeegee = group('Squeegee')
    box('Squeegee carriage', (0, 1.51, .95), (1.20, .06, .08), metal, squeegee)
    box('Squeegee blade', (0, 1.465, .95), (1.05, .05, .02), trim, squeegee, .002)
    clamp = group('Clamp')
    for x in (-.66, .66):
        box('Board clamp', (x, 1.34, .95), (.10, .10, .10), trim, clamp)


def mounter_parts(mats):
    _, trim, _, metal, _, _ = mats
    gantry = group('Gantry_X')
    box('Gantry beam', (0, 1.66, .95), (1.50, .09, .08), metal, gantry)
    rail = group('Gantry_Y')
    box('Gantry rail', (0, 1.50, .95), (.06, .42, .06), metal, rail, .004)
    head = group('Placement_Head')
    box('Head body', (0, 1.54, .95), (.20, .28, .12), trim, head)
    box('Head nozzle', (0, 1.36, .95), (.05, .12, .05), metal, head, .003)
    feeders = group('Feeders')
    box('Feeder rack', (0, 1.28, 1.00), (1.50, .10, .16), trim, feeders)
    for i in range(8):
        box('Feeder slot', (-.61 + .20 * i, 1.33, 1.02), (.09, .05, .10), metal, feeders, .002)


def camera_parts(mats):
    _, trim, glass, metal, _, _ = mats
    bridge = group('Camera_Bridge')
    box('Bridge beam', (0, 1.62, .95), (1.45, .08, .10), metal, bridge)
    head = group('Camera_Head')
    box('Camera body', (0, 1.49, .95), (.16, .18, .12), trim, head)
    box('Camera lens', (0, 1.39, .95), (.09, .09, .09), glass, head, .004)


def reflow_parts(mats):
    _, trim, _, metal, _, _ = mats
    tunnel = group('Tunnel')
    for z in (-.52, .52):
        box('Tunnel wall', (0, 1.60, z), (2.05, .55, .09), metal, tunnel)
    box('Tunnel roof', (0, 1.92, 0), (2.05, .10, .94), trim, tunnel)
    shutters = group('Zone_Shutter')
    for x in (-.45, 0, .45):
        box('Zone shutter', (x, 1.55, .95), (.06, .30, .02), metal, shutters, .002)
    belt = group('Belt')
    for i in range(6):
        box('Belt cleat', (-.90 + .36 * i, 1.30, .95), (.05, .05, .46), trim, belt, .002)


def loader_parts(mats):
    _, trim, _, metal, _, _ = mats
    magazine = group('Magazine')
    for x in (-.45, .45):
        box('Magazine wall', (x, 1.30, .96), (.06, .62, .18), metal, magazine)
    box('Magazine back', (0, 1.30, .90), (.92, .62, .04), trim, magazine)
    lift = group('Lift')
    box('Lift platform', (0, 1.10, .96), (.86, .05, .20), metal, lift)
    box('Lift post', (0, .95, .90), (.07, .35, .06), trim, lift, .004)
    box('Pusher bar', (0, 1.30, 1.06), (.90, .06, .04), trim, group('Pusher'), .003)


def build(kind):
    reset()
    mats = (
        material('Powder coated warm grey', (.68, .72, .69), .27, .48),
        material('Dark graphite', (.13, .17, .16), .38, .50),
        material('Smoked protective window', (.10, .18, .17), .25, .28),
        material('Satin aluminium', (.48, .55, .51), .75, .37),
        material('Safety amber', (.65, .35, .10), .20, .48),
        material('Plaque white', (.82, .84, .80), .10, .62),
    )
    shell(mats)
    {'printer': printer_parts, 'mounter': mounter_parts, 'spi': camera_parts,
     'aoi': camera_parts, 'reflow': reflow_parts, 'loader': loader_parts,
     'unloader': loader_parts}[kind](mats)
    bpy.context.scene['provenance'] = f'Original generic conceptual SMT machine ({kind}); no proprietary geometry.'
    source = ROOT / 'assets' / 'blender' / f'{kind}.blend'
    output = ROOT / 'public' / 'models' / f'{kind}.glb'
    output.parent.mkdir(parents=True, exist_ok=True)
    bpy.ops.wm.save_as_mainfile(filepath=str(source), compress=False)
    bpy.ops.export_scene.gltf(filepath=str(output), export_format='GLB', export_yup=True, export_apply=True)


for model in KINDS:
    build(model)
