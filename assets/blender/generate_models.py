"""Create original conceptual SMT assets. Run with Blender 4.2+ in background mode.

    blender -b --factory-startup --python assets/blender/generate_models.py

Coordinates below are in scene space (x, vertical y, depth z), metres.
No third-party reference geometry, branding, or external textures are used.
Anchor node names are a contract with src/data/anchors.ts; keep them stable.
"""
from pathlib import Path
import argparse
import math
import sys
import bpy
from mathutils import Vector

ROOT = Path(__file__).resolve().parents[2]
KINDS = ['loader', 'printer', 'spi', 'mounter', 'reflow', 'aoi', 'unloader']
# Stations with the detailed glazed enclosure and operator panel.
DETAILED = ('mounter', 'printer', 'spi', 'aoi', 'reflow', 'loader', 'unloader')
LABELS = {'mounter': 'Mounter', 'printer': 'Printer', 'spi': 'SPI', 'aoi': 'AOI', 'reflow': 'Reflow',
          'loader': 'Loader', 'unloader': 'Unloader'}
# Anchor name -> (horizontal travel, lift) in metres, mirroring src/scene/anchors.ts.
CLEARANCE = {
    'mounter': {'Gantry_X': (.45, 0), 'Gantry_Y': (.45, 0),
                'Placement_Head': (.45, -.12), 'Feeders': (0, .01)},
    'printer': {'Squeegee': (.48, 0), 'Clamp': (0, .02)},
    'spi': {'Camera_Bridge': (.57, 0), 'Camera_Head': (0, -.05)},
    'aoi': {'Camera_Bridge': (.57, 0), 'Camera_Head': (0, -.05)},
    'reflow': {'Belt': (.36, 0), 'Zone_Shutter': (0, .03)},
    # (x travel, y lift, z travel, floor): the loader's lift post passes through the cabinet.
    'loader': {'Lift': (0, .18, 0, .70), 'Pusher': (0, .10, .10)},
    'unloader': {'Lift': (0, .18, 0, .70), 'Pusher': (0, .10, .10)},
}


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


def shell(mats, kind):
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
    if kind in DETAILED:
        detailed_enclosure(mats, cabinet, kind)
    else:
        box('Upper housing', (0, 1.48, 0), (2.48, .82, 1.76), shell_mat, cabinet)
        box('Front window frame', (0, 1.55, .90), (1.84, .49, .05), trim, cabinet)
        box('Front safety glazing', (0, 1.55, .936), (1.70, .37, .022), glass, cabinet, .004)
        box('Control bezel', (.98, 1.56, .93), (.27, .34, .055), trim, cabinet)
        box('Control display', (.98, 1.60, .965), (.19, .16, .018), glass, cabinet, .003)
        box('Upper aluminium sill', (0, 1.18, .93), (2.32, .07, .08), metal, cabinet)
        for x in (-1.07, 1.07):
            box('Corner column', (x, 1.55, .91), (.10, .67, .09), metal, cabinet)
    for x in (-.55, .55):
        if kind == 'reflow':
            box('Roof vent', (x, 2.039, -.28), (.62, .006, .10), trim, cabinet, .001)
        elif kind in DETAILED:
            box('Roof vent', (x, 1.899, -.28), (.62, .006, .10), trim, cabinet, .001)
        else:
            box('Roof vent', (x, 1.892, -.28), (.62, .006, .10), trim, cabinet, .001)
    box('Nameplate', (0, .30, .945), (.58, .11, .018), plaque, cabinet, .002)
    if kind not in DETAILED:
        box('Warning plaque', (-.86, 1.52, .958), (.17, .17, .012), plaque, cabinet, .002)

    box('Conveyor bed', (0, 1.01, 0), (3.13, .12, .72), trim, conveyor)
    box('Conveyor belt', (0, 1.075, 0), (3.05, .012, .52), trim, conveyor, .001)
    for z in (-.43, .43):
        box('Conveyor rail', (0, 1.12, z), (3.13, .07, .07), metal, conveyor)

    box('Tower mast', (1.01, 1.94, -.65), (.07, .38, .07), metal, tower, .004)
    box('Tower lens', (1.01, 2.17, -.65), (.12, .16, .12), amber, lens, .004)
    box('Tower cap', (1.01, 2.26, -.65), (.14, .03, .14), trim, tower, .002)

    group('PCB_Anchor').location = (0, -.12, 1.08)


def round_button(name, position, radius, depth, mat, parent):
    """Cylinder facing the operator (+scene Z)."""
    x, y, z = position
    bpy.ops.mesh.primitive_cylinder_add(vertices=24, radius=radius, depth=depth,
                                      location=(x, -z, y), rotation=(math.pi / 2, 0, 0))
    obj = bpy.context.object
    obj.name, obj.parent = name, parent
    obj.data.materials.append(mat)
    mod = obj.modifiers.new('Button rim', 'BEVEL')
    mod.width, mod.segments = .002, 2
    bpy.ops.object.modifier_apply(modifier=mod.name)
    return obj


def panel_text(body, position, size, mat, parent):
    x, y, z = position
    curve = bpy.data.curves.new('Panel lettering', 'FONT')
    curve.body, curve.size, curve.align_x = body, size, 'CENTER'
    curve.extrude = 0
    obj = bpy.data.objects.new(body, curve)
    bpy.context.scene.collection.objects.link(obj)
    obj.location, obj.rotation_euler = (x, -z, y), (math.pi / 2, 0, 0)
    obj.parent = parent
    curve.materials.append(mat)
    bpy.ops.object.select_all(action='DESELECT')
    obj.select_set(True)
    bpy.context.view_layer.objects.active = obj
    bpy.ops.object.convert(target='MESH')


def station_finish(label, glass_name):
    """Hardware materials shared by the detailed stations; glass carries its own alpha."""
    glass = material(glass_name, (.20, .29, .27), 0, .24)
    glass.diffuse_color = (.20, .29, .27, .20)
    glass.node_tree.nodes.get('Principled BSDF').inputs['Alpha'].default_value = .20
    glass.surface_render_method = 'BLENDED'
    return {
        'rubber': material(f'{label} rubber seals', (.018, .024, .022), 0, .85),
        'glass': glass,
        'screen': material(f'{label} HMI background', (.016, .041, .046), 0, .48),
        'ink': material(f'{label} HMI ink', (.36, .68, .57), 0, .65),
        'red': material(f'{label} stop red', (.62, .025, .012), 0, .40),
        'yellow': material(f'{label} stop collar', (.90, .61, .015), 0, .60),
        'green': material(f'{label} start green', (.025, .36, .11), 0, .42),
    }


def vision_door(mats, finish, cabinet, name, center_x, center_y, width, height, hinge_side, hardware):
    """Framed, sealed door leaf. `hardware` holds the absolute hinge/handle rows for this height."""
    paint, trim, _, metal, _, _ = mats
    half = width / 2
    door = group(name, cabinet)
    for dx in (-(half - .024), half - .024):
        box('Door upright', (center_x + dx, center_y, .97), (.048, height, .065), trim, door, .004)
        box('Seal upright', (center_x + dx * .93, center_y, .98), (.014, height - .08, .018), finish['rubber'], door, .002)
    for dy in (-(height / 2 - .02), height / 2 - .02):
        box('Door crosspiece', (center_x, center_y + dy, .97), (width, .04, .065), trim, door, .004)
        box('Seal crosspiece', (center_x, center_y + dy + (.028 if dy < 0 else -.028), .98), (width - .092, .012, .018), finish['rubber'], door, .002)
    box('Safety glazing', (center_x, center_y, .974), (width - .098, height - .088, .008), finish['glass'], door, 0)
    edge = center_x + (-(half - .011) if hinge_side == 'left' else half - .011)
    for offset in hardware['hinges']:
        box('Hinge leaves', (edge, center_y + offset, 1.017), (.075, .064, .014), metal, door, .003)
        box('Hinge barrel', (edge, center_y + offset, 1.03), (.025, .078, .025), trim, door, .004)
    handle_x = center_x + ((half - .101) if hinge_side == 'left' else -(half - .101))
    for offset in hardware['handle_feet']:
        box('Handle foot', (handle_x, center_y + offset, 1.035), (.026, .027, .06), trim, door, .004)
    box('Raised vertical handle', (handle_x, center_y + hardware['handle_center'], 1.072), (.023, hardware['handle_length'], .026), metal, door, .006)
    return door


def operator_panel(mats, finish, cabinet, x, title, rows):
    """Side-mounted HMI and physical controls; content is a static illustration."""
    paint, trim, _, metal, _, white = mats
    panel = group('Operator_Panel', cabinet)
    box('Operator housing', (x, 1.51, .96), (.40, .66, .12), paint, panel, .012)
    display = group('HMI_Display', panel)
    box('HMI gasket', (x, 1.65, 1.027), (.327, .275, .012), finish['rubber'], display, .008)
    box('HMI bezel', (x, 1.65, 1.041), (.304, .253, .026), trim, display, .009)
    box('HMI screen', (x, 1.65, 1.056), (.260, .204, .005), finish['screen'], display, .002)
    panel_text('SIMULATED', (x, 1.715, 1.061), .027, white, display)
    for i, width in enumerate(rows):
        box('HMI status row', (x, 1.676 - i * .035, 1.061), (width, .009, .002), finish['ink'], display, 0)
    panel_text(title, (x, 1.563, 1.061), .019, white, display)
    for offset, label, mat in ((-.11, 'START', finish['green']), (0, 'STOP', trim), (.11, 'RESET', white)):
        round_button('Button socket', (x + offset, 1.44, 1.034), .029, .022, metal, panel)
        round_button('Button cap', (x + offset, 1.44, 1.049), .022, .018, mat, panel)
        panel_text(label, (x + offset, 1.482, 1.025), .018, trim, panel)
    stop = group('Emergency_Stop', panel)
    round_button('Yellow collar', (x, 1.295, 1.03), .054, .012, finish['yellow'], stop)
    round_button('Stop stem', (x, 1.295, 1.055), .023, .044, trim, stop)
    round_button('Mushroom stop', (x, 1.295, 1.084), .037, .023, finish['red'], stop)
    panel_text('EMERGENCY', (x, 1.218, 1.025), .019, trim, panel)
    return panel


# Door hardware rows, in metres from the leaf centre. The mounter's are the accepted sample.
HARDWARE = {
    'mounter': {'hinges': (-.21, .20), 'handle_feet': (-.10, .07), 'handle_center': -.015, 'handle_length': .19},
    'printer': {'hinges': (-.20, .19), 'handle_feet': (-.095, .065), 'handle_center': -.014, 'handle_length': .18},
}
HARDWARE['spi'] = HARDWARE['aoi'] = HARDWARE['printer']

# Per-station opening layout: named door leaves and the shared operator panel.
LAYOUT = {
    'mounter': {'doors': (('Vision_Door_L', -.685, 'left'), ('Vision_Door_R', .245, 'right')),
                'width': .922, 'height': .66, 'center_y': 1.51, 'title': 'SMT / DEMO',
                'rows': (.19, .13, .16)},
    'printer': {'doors': (('Vision_Door_L', -.68, 'left'), ('Vision_Door_R', .228, 'right')),
                'width': .90, 'height': .62, 'center_y': 1.48, 'title': 'PRINT / DEMO',
                'rows': (.19, .13, .16)},
    'spi': {'doors': (('Vision_Door', -.23, 'left'),),
            'width': 1.80, 'height': .62, 'center_y': 1.50, 'title': 'SPI / DEMO',
            'rows': (.10, .22, .16)},
    'aoi': {'doors': (('Vision_Door', -.23, 'left'),),
            'width': 1.80, 'height': .62, 'center_y': 1.50, 'title': 'AOI / DEMO',
            'rows': (.22, .10, .16)},
    'reflow': {'doors': (), 'covers': (('Maintenance_Cover_A', -.78), ('Maintenance_Cover_B', .04)),
               'width': .78, 'height': .58, 'center_y': 1.52, 'title': 'REFLOW / DEMO',
               'rows': (.16, .20, .12), 'window': (.30, .16), 'window_offset': -.18},
    'loader': {'doors': (), 'covers': (('Magazine_Door', 0),),
               'width': 1.05, 'height': .52, 'center_y': 1.44, 'title': 'LOAD / DEMO',
               'rows': (.20, .12, .16), 'window': (.34, .30)},
    'unloader': {'doors': (), 'covers': (('Magazine_Door', 0),),
                 'width': 1.05, 'height': .52, 'center_y': 1.44, 'title': 'UNLOAD / DEMO',
                 'rows': (.12, .18, .16), 'window': (.34, .30)},
}


def service_cover(mats, finish, cabinet, name, center_x, center_y, width, height, hinge_side, window, window_offset=0):
    """Insulated clad cover with a small framed inspection aperture."""
    paint, trim, _, metal, _, _ = mats
    window_width, window_height = window
    window_y = center_y + window_offset
    cover = group(name, cabinet)
    top_window = window_y + window_height / 2
    bottom_window = window_y - window_height / 2
    for dx in (-(window_width / 2 + (width - window_width) / 4), window_width / 2 + (width - window_width) / 4):
        box('Cover panel', (center_x + dx, center_y, .965), ((width - window_width) / 2, height, .03), paint, cover, .004)
    box('Cover panel', (center_x, (top_window + center_y + height / 2) / 2, .965), (window_width, center_y + height / 2 - top_window, .03), paint, cover, .004)
    box('Cover panel', (center_x, (bottom_window + center_y - height / 2) / 2, .965), (window_width, bottom_window - (center_y - height / 2), .03), paint, cover, .004)
    for dx in (-(window_width / 2 + .012), window_width / 2 + .012):
        box('Window frame', (center_x + dx, window_y, .967), (.024, window_height + .048, .012), trim, cover, .003)
    for dy in (-(window_height / 2 + .012), window_height / 2 + .012):
        box('Window frame', (center_x, window_y + dy, .967), (window_width, .024, .012), trim, cover, .003)
    box('Cover glazing', (center_x, window_y, .962), (window_width, window_height, .008), finish['glass'], cover, 0)
    edge = center_x + (-(width / 2 - .02) if hinge_side == 'left' else width / 2 - .02)
    for offset in (-.18, .18):
        box('Cover hinge', (edge, center_y + offset, .99), (.07, .06, .014), metal, cover, .003)
    latch_x = center_x + ((width / 2 - .06) if hinge_side == 'left' else -(width / 2 - .06))
    box('Cover latch', (latch_x, center_y, .995), (.06, .10, .02), trim, cover, .004)
    return cover


def detailed_enclosure(mats, cabinet, kind):
    """Hollow upper enclosure with glazed doors and a side operator panel."""
    paint, _, _, metal, _, _ = mats
    finish = station_finish(LABELS[kind], f'{kind} safety glass' if kind != 'mounter' else 'Mounter safety glass')
    layout = LAYOUT[kind]
    if kind == 'reflow':
        # The oven is insulated cladding first: a taller shell and two small inspection apertures.
        box('Enclosure roof', (0, 2.00, 0), (2.48, .07, 1.90), paint, cabinet)
        box('Enclosure back', (0, 1.52, -.87), (2.48, .96, .06), paint, cabinet)
        for x in (-1.205, 1.205):
            box('Enclosure side', (x, 1.52, 0), (.07, .96, 1.80), paint, cabinet)
        box('Window sill', (0, 1.15, .96), (2.46, .08, .10), metal, cabinet)
        box('Cladding header', (0, 1.90, .965), (2.34, .16, .03), paint, cabinet, .004)
        box('Cladding base', (0, 1.19, .965), (2.34, .08, .03), paint, cabinet, .004)
        box('Cladding infill', (.595, 1.52, .965), (.33, .58, .03), paint, cabinet, .004)
    else:
        box('Enclosure roof', (0, 1.86, 0), (2.48, .07, 1.90), paint, cabinet)
        box('Enclosure back', (0, 1.46, -.87), (2.48, .73, .06), paint, cabinet)
        for x in (-1.205, 1.205):
            box('Enclosure side', (x, 1.46, 0), (.07, .73, 1.80), paint, cabinet)
        box('Window sill', (0, 1.15, .96), (2.46, .08, .10), metal, cabinet)

    for name, center, hinge_side in layout['doors']:
        vision_door(mats, finish, cabinet, name, center, layout['center_y'],
                    layout['width'], layout['height'], hinge_side, HARDWARE[kind])
    for name, center in layout.get('covers', ()):
        service_cover(mats, finish, cabinet, name, center, layout['center_y'],
                      layout['width'], layout['height'], 'left' if center < 0 else 'right',
                      layout.get('window', (.30, .16)), layout.get('window_offset', 0))
    if kind in ('loader', 'unloader'):
        # Close the front on both sides of the guard door and above it, up to the operator panel.
        top = layout['center_y'] + layout['height'] / 2
        box('Cladding header', (0, (top + 1.825) / 2, .965), (2.34, 1.825 - top, .03), paint, cabinet, .004)
        door_left = layout['covers'][0][1] - layout['width'] / 2
        door_right = layout['covers'][0][1] + layout['width'] / 2
        box('Cladding infill', ((-1.17 + door_left) / 2, layout['center_y'], .965),
            (door_left + 1.17, layout['height'], .03), paint, cabinet, .004)
        box('Cladding infill', ((door_right + .76) / 2, layout['center_y'], .965),
            (.76 - door_right, layout['height'], .03), paint, cabinet, .004)
    operator_panel(mats, finish, cabinet, .96, layout['title'], layout['rows'])
    return finish



def consolidate():
    """Join static subparts within each parent, keeping glass and motion anchors separate."""
    buckets = {}
    for obj in list(bpy.context.scene.objects):
        if obj.type == 'MESH':
            is_glass = any(mat.name.lower().endswith('safety glass') for mat in obj.data.materials)
            buckets.setdefault((obj.parent, is_glass), []).append(obj)
    for (parent, is_glass), objects in buckets.items():
        bpy.ops.object.select_all(action='DESELECT')
        for obj in objects:
            obj.select_set(True)
        bpy.context.view_layer.objects.active = objects[0]
        if len(objects) > 1:
            bpy.ops.object.join()
        objects[0].name = f'{parent.name} {"glazing" if is_glass else "geometry"}'


def validate_clearance(kind, limits):
    """Check world-space envelope at all travel extrema before exporting."""
    bpy.context.view_layer.update()
    for name, values in limits.items():
        travel, lift = values[0], values[1]
        depth = values[2] if len(values) > 2 else 0
        floor = values[3] if len(values) > 3 else 1.09
        anchor = bpy.data.objects[name]
        for obj in anchor.children_recursive:
            if obj.type != 'MESH':
                continue
            for corner in obj.bound_box:
                point = obj.matrix_world @ Vector(corner)
                x, y, z = point.x, point.z, -point.y
                for dx in (-travel, travel):
                    for dy in (0, lift):
                        for dz in ((-depth, depth) if depth else (0,)):
                            assert -1.17 < x + dx < 1.17, f'{name} crosses a side wall'
                            assert floor < y + dy < 1.82, f'{name} crosses roof/conveyor'
                            assert -.83 < z + dz < .95, f'{name} crosses back/door'
    print(f'{kind} motion envelope: clear of enclosure at all extrema')


def world_bounds(objects):
    bpy.context.view_layer.update()
    return [obj.matrix_world @ Vector(corner) for obj in objects if obj.type == 'MESH' for corner in obj.bound_box]


def validate_door_seams(kind):
    """Two leaves must read as two doors: a real seam, not one merged mullion."""
    if 'Vision_Door_L' not in bpy.data.objects:
        print(f'{kind} has no paired vision doors: no leaf seam to check')
        return
    left = bpy.data.objects['Vision_Door_L']
    right = bpy.data.objects['Vision_Door_R']
    left_inner = max(point.x for point in world_bounds(left.children_recursive))
    right_inner = min(point.x for point in world_bounds(right.children_recursive))
    gap = right_inner - left_inner
    assert gap >= .006, f'{kind} door leaves overlap or touch (gap {gap * 1000:.1f} mm)'
    print(f'{kind} door seam between leaves: {gap * 1000:.1f} mm')


def validate_window_reveal(kind):
    """The aperture must show the moving part, not the frame or a side wall."""
    glass = [obj for obj in bpy.context.scene.objects
             if obj.type == 'MESH' and any(mat.name.lower().endswith('safety glass') for mat in obj.data.materials)]
    if not glass:
        return
    low = min(point.z for point in world_bounds(glass))
    high = max(point.z for point in world_bounds(glass))
    left = min(point.x for point in world_bounds(glass))
    right = max(point.x for point in world_bounds(glass))
    if kind == 'reflow':
        belt = bpy.data.objects['Belt']
        centre = (min(point.z for point in world_bounds(belt.children_recursive))
                  + max(point.z for point in world_bounds(belt.children_recursive))) / 2
        assert low < centre < high, f'reflow belt line {centre:.3f} is outside the window band {low:.3f}..{high:.3f}'
        print(f'reflow belt line {centre:.3f} m sits inside the aperture {low:.3f}..{high:.3f} m')
    if kind in ('loader', 'unloader'):
        magazine = bpy.data.objects['Magazine']
        centre = (min(point.x for point in world_bounds(magazine.children_recursive))
                  + max(point.x for point in world_bounds(magazine.children_recursive))) / 2
        window = (left + right) / 2
        assert abs(window - centre) < .25, f'{kind} window centre {window:.3f} is off the magazine centre {centre:.3f}'
        print(f'{kind} window centre {window:.3f} m is within 0.25 m of the magazine centre')


def validate_roof_vents(kind):
    """Vents must stand proud of the roof, not share its plane."""
    roof_top = max(point.z for point in world_bounds([bpy.data.objects['Enclosure roof']]))
    vents = [obj for obj in bpy.context.scene.objects if obj.name.startswith('Roof vent')]
    assert vents, f'{kind} roof has no vents'
    vent_top = max(point.z for point in world_bounds(vents))
    assert vent_top > roof_top + .001, f'{kind} roof vents are coplanar with the roof ({vent_top:.4f} vs {roof_top:.4f})'
    print(f'{kind} roof vents stand {(vent_top - roof_top) * 1000:.1f} mm proud of the roof')


def printer_parts(mats):
    _, trim, _, metal, _, _ = mats
    cabinet = group('Cabinet')
    box('Printer worktable', (0, 1.25, .60), (1.55, .09, .12), metal, cabinet)
    stencil = group('Stencil')
    box('Stencil frame', (0, 1.43, .60), (1.45, .02, .36), metal, stencil, .004)
    box('Stencil sheet', (0, 1.445, .60), (1.30, .006, .30), metal, stencil, .001)
    squeegee = group('Squeegee')
    box('Squeegee carriage', (0, 1.51, .60), (1.20, .06, .08), metal, squeegee)
    box('Squeegee blade', (0, 1.465, .60), (1.05, .05, .02), metal, squeegee, .002)
    clamp = group('Clamp')
    for x in (-.66, .66):
        box('Board clamp', (x, 1.34, .60), (.10, .10, .10), metal, clamp)


def mounter_parts(mats):
    _, trim, _, metal, _, _ = mats
    gantry = group('Gantry_X')
    box('Gantry beam', (0, 1.66, .50), (1.32, .09, .08), metal, gantry)
    rail = group('Gantry_Y')
    box('Gantry rail', (0, 1.50, .57), (.06, .42, .06), metal, rail, .004)
    head = group('Placement_Head')
    box('Head body', (0, 1.60, .65), (.20, .28, .12), trim, head)
    box('Head nozzle', (0, 1.42, .65), (.05, .12, .05), metal, head, .003)
    feeders = group('Feeders')
    box('Feeder rack', (0, 1.23, .76), (1.65, .10, .16), trim, feeders)
    for i in range(8):
        box('Feeder slot', (-.70 + .20 * i, 1.30, .76), (.09, .05, .10), metal, feeders, .002)


def camera_parts(mats, kind):
    _, trim, glass, metal, _, _ = mats
    bridge = group('Camera_Bridge')
    box('Bridge beam', (0, 1.62, .60), (1.16, .08, .10), metal, bridge)
    head = group('Camera_Head')
    box('Camera body', (0, 1.49, .60), (.16, .18, .12), trim, head)
    box('Camera lens', (0, 1.39, .60), (.09, .09, .09), glass, head, .004)


def reflow_parts(mats):
    _, trim, _, metal, _, _ = mats
    tunnel = group('Tunnel')
    # No front tunnel wall: the insulated cladding covers are the oven's front face.
    box('Tunnel back wall', (0, 1.60, -.52), (2.05, .55, .09), metal, tunnel)
    box('Tunnel liner', (0, 1.52, -.455), (1.95, .60, .02), trim, tunnel, .002)
    box('Tunnel roof', (0, 1.92, 0), (2.05, .10, .94), trim, tunnel)
    shutters = group('Zone_Shutter')
    for x in (-.45, 0, .45):
        box('Zone shutter', (x, 1.55, .60), (.06, .30, .02), metal, shutters, .002)
    belt = group('Belt')
    for i in range(5):
        box('Belt cleat', (-.72 + .36 * i, 1.30, .02), (.05, .05, .46), trim, belt, .002)


def loader_parts(mats):
    _, trim, _, metal, _, _ = mats
    magazine = group('Magazine')
    for x in (-.45, .45):
        box('Magazine wall', (x, 1.30, .48), (.06, .62, .18), metal, magazine)
    box('Magazine back', (0, 1.30, .42), (.92, .62, .04), trim, magazine)
    for i in range(3):
        box('Magazine shelf', (0, 1.12 + i * .18, .48), (.84, .012, .16), metal, magazine, .002)
    lift = group('Lift')
    box('Lift platform', (0, 1.10, .48), (.86, .05, .20), metal, lift)
    box('Lift post', (0, .95, .42), (.07, .35, .06), trim, lift, .004)
    box('Pusher bar', (0, 1.30, .62), (.90, .06, .04), trim, group('Pusher'), .003)


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
    shell(mats, kind)
    if kind in ('spi', 'aoi'):
        camera_parts(mats, kind)
    else:
        {'printer': printer_parts, 'mounter': mounter_parts, 'reflow': reflow_parts,
         'loader': loader_parts, 'unloader': loader_parts}[kind](mats)
    if kind in DETAILED:
        validate_clearance(kind, CLEARANCE[kind])
        validate_door_seams(kind)
        validate_window_reveal(kind)
        validate_roof_vents(kind)
        consolidate()
    bpy.context.scene['provenance'] = f'Original generic conceptual SMT machine ({kind}); no proprietary geometry.'
    source = ROOT / 'assets' / 'blender' / f'{kind}.blend'
    output = ROOT / 'public' / 'models' / f'{kind}.glb'
    output.parent.mkdir(parents=True, exist_ok=True)
    bpy.ops.wm.save_as_mainfile(filepath=str(source), compress=False)
    bpy.ops.export_scene.gltf(filepath=str(output), export_format='GLB', export_yup=True, export_apply=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--kind', choices=KINDS)
    args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:] if '--' in sys.argv else [])
    for model in ([args.kind] if args.kind else KINDS):
        build(model)
