export interface Account {
  id: string
  name: string
  qq_uin: string
  ws_url: string
  http_url: string
  enabled: boolean
  status: string
  last_connected_at?: string
  created_at: string
}

export interface QZoneConnection {
  id: string
  account_id: string
  http_url: string
  ws_url: string
  enabled: boolean
  status: string
  last_connected_at?: string
  last_event_at?: string
  last_error?: string
}

export interface Capability {
  endpoint: string
  method: string
  summary: string
  tag?: string
  read_only: boolean
  requires_confirmation: boolean
  requires_admin: boolean
  implemented: boolean
}

export interface Overview {
  accounts: number
  raw_records: number
  messages: number
  relation_events: number
  contents?: number
}

export interface RealtimeStats {
  current_per_minute: number
  last_5_minutes: number
  total_realtime_messages: number
  connected_accounts: number
  last_message_at?: string
  series: Array<{ time: string; count: number }>
}

export interface MediaStats {
  pending: number
  downloading: number
  completed: number
  failed: number
  assets: number
  bytes: number
}

export interface ConversationItem {
  id: string
  conversation_type: 'group' | 'private'
  platform_conversation_id: string
  title: string
  avatar_uri: string
  message_count: number
  last_sent_at?: string
  last_message?: {
    id: string
    text: string
    sender_name: string
    sender_qq: string
    sent_at?: string
  }
}

export interface ChatMessage {
  id: string
  source_message_id?: string
  sender: string
  sender_qq: string
  sender_avatar?: string
  text: string
  segments?: any[]
  sent_at?: string
  reply_to?: string
  reply_sender?: string
  reply_text?: string
  is_recalled?: boolean
  raw_record_id?: string
  media?: Array<{
    reference_id: string
    kind: string
    status: string
    asset_url: string
    mime_type: string
    filename: string
  }>
}

export interface ActiveMember {
  person_id: string
  qq: string
  display_name: string
  avatar_uri: string
  message_count: number
  last_active_at?: string
  role?: string
}

export interface MediaItem {
  id: string
  reference_id: string
  message_id: string
  kind: string
  status: string
  asset_url: string
  mime_type: string
  filename: string
  sent_at?: string
  sender_name: string
}

export interface ConversationContext {
  conversation: ConversationItem
  active_rank: ActiveMember[]
  media_items: MediaItem[]
  total_media: number
  total_members: number
}
