export type Vec3 = [number, number, number]
export type Station = { id: string; name: string; short: string; description: string; position: Vec3; bounds: Vec3; camera: { position: Vec3; target: Vec3 }; kind: string }
const entries = [
  ['loader', 'PCB Loader', 'LOAD', 'Introduces bare boards to the line.', 'loader'],
  ['printer', 'Solder Paste Printer', 'PRINT', 'Conceptual stencil printing station, inspired by DEK equipment.', 'printer'],
  ['spi', 'Solder Paste Inspection', 'SPI', 'Inspects the deposited solder paste before placement.', 'inspection'],
  ['mounter', 'Pick & Place', 'PLACE', 'Conceptual component placement station, inspired by SIPLACE equipment.', 'mounter'],
  ['reflow', 'Reflow Oven', 'REFLOW', 'Generic thermal process for soldering placed components.', 'oven'],
  ['aoi', 'Automated Optical Inspection', 'AOI', 'Generic optical inspection of assembled boards.', 'inspection'],
  ['unloader', 'PCB Unloader', 'UNLOAD', 'Collects finished boards at the end of the line.', 'loader'],
]
export const stations: Station[] = entries.map(([id, name, short, description, kind], i) => {
  const x = (i - 3) * 3.15
  return { id, name, short, description, kind, position: [x, 0, 0], bounds: [2.6, 2.5, 2], camera: { position: [x + 3.6, 3.2, 5.3], target: [x, 1.05, 0] } }
})
export const overviewCamera: { position: Vec3; target: Vec3 } = { position: [14, 12, 22], target: [0, 0.7, 0] }
export const boardPositions: Vec3[] = Array.from({ length: 20 }, (_, i) => [-10 + i * 1.05, 1.08, 0])
