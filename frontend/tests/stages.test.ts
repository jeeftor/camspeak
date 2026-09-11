import { expect, test } from 'bun:test'
import { describeStages, stageInfo, stageColor } from '../src/lib/stages'
import { stepLabel } from '../src/lib/utils'

test('pipeline stage labels share the same registry as timing summaries', () => {
  for (const stage of describeStages) {
    expect(stepLabel(stage.timing)).toBe(stage.label)
    expect(stageInfo(stage.stage).description).toBe(stage.description)
    expect(stageColor(stage.stage)).toBe(stageColor(stage.timing))
  }
  expect(stepLabel('snap_ms')).toBe('Snapshot')
  expect(stepLabel('send_playback_ms')).toBe('Playback')
})

test('vision first token is explicitly different from first audio and model loading', () => {
  expect(stageInfo('ttft_ms').label).toBe('First token (TTFT)')
  expect(stageInfo('ttft_ms').description).toContain('Model loading is not measured separately')
  expect(stageInfo('ttfs_ms').label).toBe('First audio sent')
  expect(stepLabel('custom_ms')).toBe('custom')
})
