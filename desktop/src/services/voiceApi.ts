import { getApiBase, getToken } from './api'

export interface VoiceChatResponse {
  transcript: string
  reply: string
  audio: string
  format: string
  tts_error?: string
}

export async function voiceChat(audioBlob: Blob): Promise<VoiceChatResponse> {
  const token = getToken()
  const form = new FormData()
  form.append('audio', audioBlob, 'voice.wav')
  form.append('format', 'wav')

  const res = await fetch(`${getApiBase()}/api/v1/voice/chat`, {
    method: 'POST',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: form,
  })

  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'voice chat failed')
  return data
}
