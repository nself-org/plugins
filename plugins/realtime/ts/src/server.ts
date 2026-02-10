#!/usr/bin/env node
/**
 * Realtime Socket.io Server
 */

import { createServer } from 'http';
import { Server, Socket } from 'socket.io';
import { createAdapter } from '@socket.io/redis-adapter';
import { createClient } from 'redis';
import Fastify from 'fastify';
import cors from '@fastify/cors';
import jwt from 'jsonwebtoken';
import pino from 'pino';
import { getAppContext, normalizeSourceAccountId } from '@nself/plugin-utils';
import { config } from './config.js';
import { Database } from './database.js';
import type {
  JoinRoomPayload,
  LeaveRoomPayload,
  MessagePayload,
  TypingPayload,
  PresencePayload,
  SocketCallback,
  ServerStats,
} from './types.js';

// =============================================================================
// Logger
// =============================================================================

const logger = pino({
  level: config.logLevel,
  transport:
    process.env.NODE_ENV !== 'production'
      ? {
          target: 'pino-pretty',
          options: {
            colorize: true,
            translateTime: 'HH:MM:ss',
            ignore: 'pid,hostname',
          },
        }
      : undefined,
});

// =============================================================================
// Database
// =============================================================================

const db = new Database({
  host: config.databaseHost,
  port: config.databasePort,
  database: config.databaseName,
  user: config.databaseUser,
  password: config.databasePassword,
  ssl: config.databaseSsl,
});

// =============================================================================
// Helpers
// =============================================================================

/**
 * Resolve source account ID from a Socket.io handshake.
 *
 * Resolution priority:
 * 1. handshake.auth.sourceAccountId (set by client)
 * 2. X-App-Name header on the upgrade request
 * 3. 'app' query parameter on the connection URL
 * 4. Falls back to 'primary'
 */
function resolveSocketAccountId(socket: Socket): string {
  // 1. Auth payload
  const authId = socket.handshake.auth?.sourceAccountId;
  if (authId && typeof authId === 'string') {
    return normalizeSourceAccountId(authId);
  }

  // 2. X-App-Name header
  const headerValue = socket.handshake.headers['x-app-name'];
  const header = Array.isArray(headerValue) ? headerValue[0] : headerValue;
  if (header) {
    return normalizeSourceAccountId(header);
  }

  // 3. Query parameter
  const queryApp = socket.handshake.query?.app;
  const queryValue = Array.isArray(queryApp) ? queryApp[0] : queryApp;
  if (queryValue && typeof queryValue === 'string') {
    return normalizeSourceAccountId(queryValue);
  }

  return 'primary';
}

// =============================================================================
// Fastify HTTP Server (for health checks and metrics)
// =============================================================================

const fastify = Fastify({ logger: false });

await fastify.register(cors, {
  origin: config.corsOrigin,
  credentials: true,
});

// Multi-app context: resolve source_account_id per HTTP request and create scoped DB
fastify.decorateRequest('scopedDb', null);
fastify.addHook('onRequest', async (request) => {
  const ctx = getAppContext(request);
  (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
});

/** Extract scoped Database from a Fastify request */
function scopedDbFromRequest(request: unknown): Database {
  return (request as Record<string, unknown>).scopedDb as Database;
}

// Health check
if (config.enableHealth) {
  fastify.get(config.healthPath, async (request) => {
    const sdb = scopedDbFromRequest(request);
    const stats = await sdb.getStats();
    return {
      status: 'healthy',
      timestamp: new Date(),
      connections: stats.connections,
      uptime: process.uptime(),
    };
  });
}

// Metrics endpoint
if (config.enableMetrics) {
  fastify.get(config.metricsPath, async (request) => {
    const sdb = scopedDbFromRequest(request);
    const stats = await sdb.getStats();
    const memUsage = process.memoryUsage();

    const serverStats: ServerStats = {
      uptime: process.uptime(),
      connections: {
        total: stats.connections,
        active: stats.connections,
        authenticated: stats.authenticatedConnections,
        anonymous: stats.connections - stats.authenticatedConnections,
      },
      rooms: {
        total: stats.rooms,
        active: stats.rooms,
      },
      presence: stats.presence,
      events: {
        total: 0,
        lastHour: stats.eventsLastHour,
      },
      memory: {
        used: memUsage.heapUsed,
        total: memUsage.heapTotal,
        percentage: (memUsage.heapUsed / memUsage.heapTotal) * 100,
      },
      cpu: {
        usage: 0,
      },
    };

    return serverStats;
  });
}

const httpServer = createServer(fastify.server);

// =============================================================================
// Socket.io Server
// =============================================================================

const io = new Server(httpServer, {
  cors: {
    origin: config.corsOrigin,
    credentials: true,
  },
  pingTimeout: config.pingTimeout,
  pingInterval: config.pingInterval,
  maxHttpBufferSize: 1e6, // 1MB
  transports: ['websocket', 'polling'],
});

// =============================================================================
// Redis Adapter
// =============================================================================

const pubClient = createClient({ url: config.redisUrl });
const subClient = pubClient.duplicate();

await Promise.all([pubClient.connect(), subClient.connect()]);

io.adapter(createAdapter(pubClient, subClient));

logger.info('Redis adapter configured');

// =============================================================================
// Authentication Middleware
// =============================================================================

io.use(async (socket, next) => {
  const token = socket.handshake.auth?.token;

  if (!token) {
    if (config.allowAnonymous) {
      logger.debug('Anonymous connection allowed');
      return next();
    } else {
      return next(new Error('Authentication required'));
    }
  }

  // Verify JWT if secret is configured
  if (config.jwtSecret) {
    try {
      const decoded = jwt.verify(token, config.jwtSecret) as { userId: string; sessionId?: string };
      socket.data.userId = decoded.userId;
      socket.data.sessionId = decoded.sessionId;
      logger.debug({ userId: decoded.userId }, 'User authenticated');
    } catch (error) {
      logger.warn({ error }, 'JWT verification failed');
      return next(new Error('Invalid token'));
    }
  } else {
    // Without JWT secret, trust the token payload (dev mode)
    try {
      const decoded = JSON.parse(Buffer.from(token.split('.')[1], 'base64').toString());
      socket.data.userId = decoded.userId;
      socket.data.sessionId = decoded.sessionId;
      logger.debug({ userId: decoded.userId }, 'User authenticated (no verification)');
    } catch (error) {
      logger.warn({ error }, 'Token parsing failed');
      return next(new Error('Invalid token'));
    }
  }

  next();
});

// =============================================================================
// App Context Middleware (Socket.io)
// =============================================================================

io.use(async (socket, next) => {
  // Resolve the source account from the handshake and store it + a scoped DB
  const accountId = resolveSocketAccountId(socket);
  socket.data.sourceAccountId = accountId;
  socket.data.scopedDb = db.forSourceAccount(accountId);
  logger.debug({ socketId: socket.id, sourceAccountId: accountId }, 'App context resolved for socket');
  next();
});

// =============================================================================
// Connection Handling
// =============================================================================

io.on('connection', async (socket: Socket) => {
  const { userId, sessionId } = socket.data;
  const sdb: Database = socket.data.scopedDb;
  const ipAddress = socket.handshake.address;
  const userAgent = socket.handshake.headers['user-agent'];
  const transport = socket.conn.transport.name as 'websocket' | 'polling';

  logger.info(
    {
      socketId: socket.id,
      userId,
      transport,
      ipAddress,
      sourceAccountId: sdb.getSourceAccountId(),
    },
    'Client connected'
  );

  // Create connection record
  await sdb.createConnection({
    socketId: socket.id,
    userId,
    sessionId,
    transport,
    ipAddress,
    userAgent,
    deviceInfo: socket.handshake.auth?.device,
  });

  // Update presence
  if (userId && config.enablePresence) {
    await sdb.upsertPresence(userId, 'online');
    await sdb.incrementConnectionCount(userId);

    // Broadcast presence change
    const presence = await sdb.getPresence(userId);
    if (presence) {
      io.emit('presence:changed', {
        userId,
        status: presence.status,
        customStatus: presence.custom_status,
        customEmoji: presence.custom_emoji,
      });
    }
  }

  // Log event
  if (config.logEvents && config.logEventTypes.includes('connect')) {
    await sdb.logEvent({
      eventType: 'connect',
      socketId: socket.id,
      userId,
      ipAddress,
    });
  }

  // Send connected confirmation
  socket.emit('connected', {
    socketId: socket.id,
    serverTime: new Date(),
    protocolVersion: '1.0',
  });

  // -------------------------------------------------------------------------
  // Room Management
  // -------------------------------------------------------------------------

  socket.on('room:join', async (payload: JoinRoomPayload, callback?: SocketCallback) => {
    try {
      logger.debug({ socketId: socket.id, room: payload.roomName }, 'Joining room');

      const room = await sdb.getRoomByName(payload.roomName);
      if (!room) {
        const error = { code: 'ROOM_NOT_FOUND', message: 'Room not found' };
        callback?.({ success: false, error });
        return;
      }

      // Join Socket.io room
      await socket.join(payload.roomName);

      // Add member to database
      if (userId) {
        await sdb.addRoomMember(room.id, userId);
      }

      // Get member count
      const members = await sdb.getRoomMembers(room.id);

      // Notify others
      socket.to(payload.roomName).emit('user:joined', {
        roomName: payload.roomName,
        userId,
      });

      callback?.({
        success: true,
        data: {
          roomName: payload.roomName,
          memberCount: members.length,
        },
      });

      logger.info({ userId, room: payload.roomName }, 'User joined room');
    } catch (error) {
      logger.error({ error }, 'Error joining room');
      callback?.({
        success: false,
        error: { code: 'INTERNAL_ERROR', message: 'Failed to join room' },
      });
    }
  });

  socket.on('room:leave', async (payload: LeaveRoomPayload, callback?: SocketCallback) => {
    try {
      logger.debug({ socketId: socket.id, room: payload.roomName }, 'Leaving room');

      const room = await sdb.getRoomByName(payload.roomName);
      if (!room) {
        callback?.({ success: false, error: { code: 'ROOM_NOT_FOUND', message: 'Room not found' } });
        return;
      }

      // Leave Socket.io room
      await socket.leave(payload.roomName);

      // Remove member from database
      if (userId) {
        await sdb.removeRoomMember(room.id, userId);
      }

      // Notify others
      socket.to(payload.roomName).emit('user:left', {
        roomName: payload.roomName,
        userId,
      });

      callback?.({ success: true });

      logger.info({ userId, room: payload.roomName }, 'User left room');
    } catch (error) {
      logger.error({ error }, 'Error leaving room');
      callback?.({
        success: false,
        error: { code: 'INTERNAL_ERROR', message: 'Failed to leave room' },
      });
    }
  });

  // -------------------------------------------------------------------------
  // Messaging
  // -------------------------------------------------------------------------

  socket.on('message:send', async (payload: MessagePayload, callback?: SocketCallback) => {
    try {
      logger.debug({ userId, room: payload.roomName }, 'Sending message');

      const room = await sdb.getRoomByName(payload.roomName);
      if (!room) {
        callback?.({ success: false, error: { code: 'ROOM_NOT_FOUND', message: 'Room not found' } });
        return;
      }

      // Broadcast message to room
      io.to(payload.roomName).emit('message:new', {
        roomName: payload.roomName,
        userId,
        content: payload.content,
        threadId: payload.threadId,
        timestamp: new Date(),
        metadata: payload.metadata,
      });

      callback?.({ success: true });

      logger.info({ userId, room: payload.roomName }, 'Message sent');
    } catch (error) {
      logger.error({ error }, 'Error sending message');
      callback?.({
        success: false,
        error: { code: 'INTERNAL_ERROR', message: 'Failed to send message' },
      });
    }
  });

  // -------------------------------------------------------------------------
  // Typing Indicators
  // -------------------------------------------------------------------------

  if (config.enableTyping) {
    socket.on('typing:start', async (payload: TypingPayload) => {
      if (!userId) return;

      const room = await sdb.getRoomByName(payload.roomName);
      if (!room) return;

      await sdb.setTyping(room.id, userId, payload.threadId);

      const typingUsers = await sdb.getTypingUsers(room.id, payload.threadId);

      socket.to(payload.roomName).emit('typing:event', {
        roomName: payload.roomName,
        threadId: payload.threadId,
        users: typingUsers.map(t => ({
          userId: t.user_id,
          startedAt: t.started_at,
        })),
      });
    });

    socket.on('typing:stop', async (payload: TypingPayload) => {
      if (!userId) return;

      const room = await sdb.getRoomByName(payload.roomName);
      if (!room) return;

      await sdb.clearTyping(room.id, userId, payload.threadId);

      const typingUsers = await sdb.getTypingUsers(room.id, payload.threadId);

      socket.to(payload.roomName).emit('typing:event', {
        roomName: payload.roomName,
        threadId: payload.threadId,
        users: typingUsers.map(t => ({
          userId: t.user_id,
          startedAt: t.started_at,
        })),
      });
    });
  }

  // -------------------------------------------------------------------------
  // Presence
  // -------------------------------------------------------------------------

  if (config.enablePresence && userId) {
    socket.on('presence:update', async (payload: PresencePayload) => {
      await sdb.upsertPresence(userId, payload.status, payload.customStatus);

      const presence = await sdb.getPresence(userId);
      if (presence) {
        io.emit('presence:changed', {
          userId,
          status: presence.status,
          customStatus: presence.custom_status,
          customEmoji: presence.custom_emoji,
        });
      }
    });

    // Presence heartbeat
    const heartbeatInterval = setInterval(async () => {
      if (userId) {
        await sdb.updatePresenceHeartbeat(userId);
      }
    }, config.presenceHeartbeat);

    socket.on('disconnect', () => {
      clearInterval(heartbeatInterval);
    });
  }

  // -------------------------------------------------------------------------
  // Ping/Pong
  // -------------------------------------------------------------------------

  socket.on('ping', async () => {
    const pingTime = Date.now();
    await sdb.updatePing(socket.id);
    socket.emit('pong', { timestamp: pingTime });
  });

  socket.on('pong', async (data: { timestamp: number }) => {
    const latency = Date.now() - data.timestamp;
    await sdb.updatePong(socket.id, latency);
  });

  // -------------------------------------------------------------------------
  // Disconnection
  // -------------------------------------------------------------------------

  socket.on('disconnect', async (reason) => {
    logger.info({ socketId: socket.id, userId, reason }, 'Client disconnected');

    await sdb.disconnectConnection(socket.id);

    if (userId && config.enablePresence) {
      await sdb.decrementConnectionCount(userId);

      const presence = await sdb.getPresence(userId);
      if (presence && presence.connections_count === 0) {
        await sdb.upsertPresence(userId, 'offline');
        io.emit('presence:changed', {
          userId,
          status: 'offline',
        });
      }
    }

    if (config.logEvents && config.logEventTypes.includes('disconnect')) {
      await sdb.logEvent({
        eventType: 'disconnect',
        socketId: socket.id,
        userId,
        payload: { reason },
      });
    }
  });

  // Error handling
  socket.on('error', (error) => {
    logger.error({ error, socketId: socket.id }, 'Socket error');
  });
});

// =============================================================================
// Background Tasks
// =============================================================================

// Clean expired typing indicators every 5 seconds
setInterval(async () => {
  await db.cleanExpiredTyping();
}, 5000);

// =============================================================================
// Graceful Shutdown
// =============================================================================

const shutdown = async () => {
  logger.info('Shutting down gracefully...');

  io.close(() => {
    logger.info('Socket.io server closed');
  });

  await Promise.all([
    db.close(),
    pubClient.quit(),
    subClient.quit(),
    fastify.close(),
  ]);

  logger.info('All connections closed');
  process.exit(0);
};

process.on('SIGTERM', shutdown);
process.on('SIGINT', shutdown);

// =============================================================================
// Start Server
// =============================================================================

httpServer.listen(config.port, config.host, () => {
  logger.info(
    {
      port: config.port,
      host: config.host,
      corsOrigin: config.corsOrigin,
    },
    'Realtime server started'
  );
});
