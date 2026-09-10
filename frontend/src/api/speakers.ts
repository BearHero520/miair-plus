import http from './http'

export interface SpeakerStatus {
  did: string
  name: string
  dlna_name: string
  hardware: string
  enabled: boolean
  compatibility_mode: boolean
  udn: string
  transport_state: string
  current_uri: string
  airplay_active: boolean
  airplay_client: string
  name_sync?: "idle" | "pending" | "synced" | "failed"
  name_sync_error?: string
}

export interface CloudDevice {
  miotDID: string
  hardware: string
  name: string
}

export async function fetchSpeakers(): Promise<SpeakerStatus[]> {
  const { data } = await http.get('/speakers')
  return data
}

export async function fetchCloudDevices(): Promise<{ devices: CloudDevice[]; error?: string }> {
  const { data } = await http.get('/devices')
  return data
}

export async function renameSpeaker(did: string, dlnaName: string) {
  const { data } = await http.post(`/speakers/${did}/rename`, { dlna_name: dlnaName })
  return data
}

// 切换音箱兼容模式 (写入配置并热重启 DLNA/AirPlay 服务)
export async function setCompatibilityMode(did: string, value: boolean) {
  const { data } = await http.post('/settings', {
    speakers: { [did]: { compatibility_mode: value } },
  })
  return data
}
