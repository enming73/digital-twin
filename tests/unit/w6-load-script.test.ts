import { execFileSync, spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('W6 load script', () => {
  it('documents a non-destructive, isolated run with --help', () => {
    const text = execFileSync('bash', ['scripts/w6-load.sh', '--help'], { encoding: 'utf8' })
    expect(text).toContain('1000')
    expect(text).toContain('volume')
  })
  it('rejects an invalid project name before starting docker', () => {
    const result = spawnSync('bash', ['scripts/w6-load.sh', 'BAD/name'], { encoding: 'utf8' })
    expect(result.status).not.toBe(0)
    expect(result.stderr).toContain('project')
  })
  it('does not start or stop an existing compose project', () => {
    const dir = mkdtempSync('/tmp/opencode/w6-script-')
    try {
      const log = join(dir, 'calls')
      writeFileSync(join(dir, 'docker'), '#!/bin/sh\nprintf "%s\\n" "$*" >> "$LOG"\ncase "$*" in *"ps -aq --all"*) printf "existing-container\\n";; esac\n', { mode: 0o755 })
      const result = spawnSync('bash', ['scripts/w6-load.sh', 'w6-existing'], { encoding: 'utf8', env: { ...process.env, PATH: `${dir}:${process.env.PATH}`, LOG: log } })
      expect(result.status).not.toBe(0)
      expect(readFileSync(log, 'utf8')).not.toMatch(/\b(up|stop)\b/)
    } finally { rmSync(dir, { recursive: true, force: true }) }
  })
})
