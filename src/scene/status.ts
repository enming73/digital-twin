/** Tower light colour for a station state; the same mapping drives procedural and GLB towers. */
export function signalColor(state: string): string {
  switch (state) {
    case 'FAULT':
      return '#e14e4e'
    case 'BLOCKED':
    case 'STARVED':
      return '#e1a545'
    case 'RUNNING':
      return '#53b879'
    default:
      return '#87928c'
  }
}

export function signalIntensity(state: string): number {
  switch (state) {
    case 'FAULT':
      return 1
    case 'BLOCKED':
    case 'STARVED':
      return 0.8
    case 'RUNNING':
      return 0.65
    default:
      return 0.12
  }
}
