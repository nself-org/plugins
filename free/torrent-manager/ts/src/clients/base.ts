/**
 * Base Torrent Client Interface
 */

import type {
  TorrentClientAdapter,
  TorrentClientType,
  TorrentDownload,
  AddTorrentOptions,
  TorrentFilter,
  TorrentClientStats,
} from '../types.js';

export abstract class BaseTorrentClient implements TorrentClientAdapter {
  abstract readonly type: TorrentClientType;

  protected host: string;
  protected port: number;
  protected username?: string;
  protected password?: string;
  protected connected: boolean = false;

  constructor(host: string, port: number, username?: string, password?: string) {
    this.host = host;
    this.port = port;
    this.username = username;
    this.password = password;
  }

  abstract connect(): Promise<boolean>;
  abstract disconnect(): Promise<void>;
  abstract isConnected(): Promise<boolean>;
  abstract addTorrent(magnetUri: string, options: AddTorrentOptions): Promise<TorrentDownload>;
  abstract getTorrent(id: string): Promise<TorrentDownload | null>;
  abstract listTorrents(filter?: TorrentFilter): Promise<TorrentDownload[]>;
  abstract pauseTorrent(id: string): Promise<void>;
  abstract resumeTorrent(id: string): Promise<void>;
  abstract removeTorrent(id: string, deleteFiles: boolean): Promise<void>;
  abstract getStats(): Promise<TorrentClientStats>;
}
