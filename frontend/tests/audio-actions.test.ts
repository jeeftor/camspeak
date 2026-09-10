import { afterEach, describe, expect, mock, spyOn, test } from 'bun:test'
import { apiClient } from '../src/lib/api'
import { uploadAudioToCamera, saveCameraGain } from '../src/lib/audio-actions'
import { cancelAudioPreparation, registerAudioPreparation } from '../src/lib/audio-preparation'
import { createAudioDraft } from '../src/lib/audio-draft'
import type { UploadJob } from '../src/lib/types'

afterEach(() => { cancelAudioPreparation(); mock.restore() })

describe('upload playback cancellation', () => {
  for (const scope of ['camera', 'all'] as const) {
    test(`Stop ${scope} prevents playback when conversion finishes later`, async () => {
      let finish!: (job: UploadJob) => void
      const conversion = new Promise<UploadJob>(resolve => { finish = resolve })
      // Deliberately ignore abort in this fake: the completion guard must still win.
      spyOn(apiClient, 'uploadAndWait').mockImplementation(() => conversion)
      const play = spyOn(apiClient, 'play').mockResolvedValue({ status: 'ok' })
      spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }))
      const upload = uploadAudioToCamera('front', new File(['audio'], 'tone.wav'), () => {}, new AbortController().signal)
      const rejected = upload.catch(error => error)
      if (scope === 'camera') await apiClient.stop('front')
      else await apiClient.stopAll()
      finish({ name: 'tone', category: 'drops', status: 'done' } as UploadJob)
      expect(await rejected).toMatchObject({ name: 'AbortError' })
      expect(play).not.toHaveBeenCalled()
    })
  }

  test('camera Stop leaves another camera preparation running', () => {
    const front = new AbortController()
    const back = new AbortController()
    const releaseFront = registerAudioPreparation('front', front)
    const releaseBack = registerAudioPreparation('back', back)
    cancelAudioPreparation('front')
    expect(front.signal.aborted).toBe(true)
    expect(back.signal.aborted).toBe(false)
    releaseFront()
    releaseBack()
  })
})

test('failed volume persistence restores the confirmed slider value', async () => {
  const draft = createAudioDraft(2)
  draft.gain = 7
  spyOn(apiClient, 'setVolume').mockRejectedValue(new Error('Camera configuration is unavailable'))
  await expect(saveCameraGain('front', draft)).rejects.toThrow('unavailable')
  expect(draft.gain).toBe(2)
  expect(draft.savedGain).toBe(2)
  expect(draft.gainSaving).toBe(false)
})
