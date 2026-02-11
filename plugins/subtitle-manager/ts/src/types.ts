export interface SubtitleManagerConfig {
  database_url: string;
  port: number;
  opensubtitles_api_key?: string;
  log_level: string;
}

export interface Subtitle {
  id: string;
  media_id: string;
  media_type: 'movie' | 'tv_episode';
  language: string;
  file_path: string;
  source: string;
  sync_score?: number;
  created_at: Date;
  updated_at: Date;
}
