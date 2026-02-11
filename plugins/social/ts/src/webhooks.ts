/**
 * Social Plugin Webhook Handler
 * Process webhook events for all social activities
 */

import { createLogger } from '@nself/plugin-utils';
import { SocialDatabase } from './database.js';
import type { WebhookEvent } from './types.js';

const logger = createLogger('social:webhooks');

export class SocialWebhookHandler {
  constructor(private db: SocialDatabase) {}

  async handle(event: WebhookEvent): Promise<void> {
    logger.info('Processing webhook event', { type: event.type });

    // Store webhook event
    await this.db.insertWebhookEvent(event.type, event.data);

    try {
      switch (event.type) {
        case 'post.created':
          await this.handlePostCreated(event.data);
          break;
        case 'post.updated':
          await this.handlePostUpdated(event.data);
          break;
        case 'post.deleted':
          await this.handlePostDeleted(event.data);
          break;
        case 'comment.created':
          await this.handleCommentCreated(event.data);
          break;
        case 'comment.updated':
          await this.handleCommentUpdated(event.data);
          break;
        case 'comment.deleted':
          await this.handleCommentDeleted(event.data);
          break;
        case 'reaction.added':
          await this.handleReactionAdded(event.data);
          break;
        case 'reaction.removed':
          await this.handleReactionRemoved(event.data);
          break;
        case 'follow.created':
          await this.handleFollowCreated(event.data);
          break;
        case 'follow.deleted':
          await this.handleFollowDeleted(event.data);
          break;
        case 'bookmark.created':
          await this.handleBookmarkCreated(event.data);
          break;
        case 'bookmark.deleted':
          await this.handleBookmarkDeleted(event.data);
          break;
        case 'share.created':
          await this.handleShareCreated(event.data);
          break;
        default:
          logger.warn('Unknown webhook event type', { type: event.type });
      }

      await this.db.markEventProcessed(`${event.type}-${Date.now()}`);
      logger.info('Webhook event processed successfully', { type: event.type });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to process webhook event', { type: event.type, error: message });
      await this.db.markEventProcessed(`${event.type}-${Date.now()}`, message);
      throw error;
    }
  }

  private async handlePostCreated(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling post.created', { data });

    const post = await this.db.createPost({
      author_id: data.author_id as string,
      content: data.content as string,
      content_type: (data.content_type as never) ?? 'text',
      attachments: (data.attachments as never) ?? [],
      visibility: (data.visibility as never) ?? 'public',
      hashtags: (data.hashtags as string[]) ?? [],
      mentions: (data.mentions as string[]) ?? [],
      location: (data.location as never) ?? null,
      metadata: (data.metadata as Record<string, unknown>) ?? {},
    });

    logger.info('Post created', { id: post.id });
  }

  private async handlePostUpdated(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling post.updated', { data });

    if (!data.id) {
      throw new Error('Post ID is required for update');
    }

    const post = await this.db.updatePost(data.id as string, {
      content: data.content as string,
      attachments: data.attachments as never,
      visibility: data.visibility as never,
      hashtags: data.hashtags as string[],
      mentions: data.mentions as string[],
      location: data.location as never,
      is_pinned: data.is_pinned as boolean,
      metadata: data.metadata as Record<string, unknown>,
    });

    if (post) {
      logger.info('Post updated', { id: post.id });
    } else {
      logger.warn('Post not found for update', { id: data.id });
    }
  }

  private async handlePostDeleted(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling post.deleted', { data });

    if (!data.id) {
      throw new Error('Post ID is required for deletion');
    }

    const deleted = await this.db.deletePost(data.id as string);

    if (deleted) {
      logger.info('Post deleted', { id: data.id });
    } else {
      logger.warn('Post not found for deletion', { id: data.id });
    }
  }

  private async handleCommentCreated(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling comment.created', { data });

    const comment = await this.db.createComment({
      target_type: data.target_type as string,
      target_id: data.target_id as string,
      parent_id: data.parent_id as string,
      author_id: data.author_id as string,
      content: data.content as string,
      mentions: (data.mentions as string[]) ?? [],
    });

    logger.info('Comment created', { id: comment.id });
  }

  private async handleCommentUpdated(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling comment.updated', { data });

    if (!data.id) {
      throw new Error('Comment ID is required for update');
    }

    const comment = await this.db.updateComment(data.id as string, {
      content: data.content as string,
      mentions: data.mentions as string[],
    });

    if (comment) {
      logger.info('Comment updated', { id: comment.id });
    } else {
      logger.warn('Comment not found for update', { id: data.id });
    }
  }

  private async handleCommentDeleted(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling comment.deleted', { data });

    if (!data.id) {
      throw new Error('Comment ID is required for deletion');
    }

    const deleted = await this.db.deleteComment(data.id as string);

    if (deleted) {
      logger.info('Comment deleted', { id: data.id });
    } else {
      logger.warn('Comment not found for deletion', { id: data.id });
    }
  }

  private async handleReactionAdded(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling reaction.added', { data });

    const reaction = await this.db.addReaction({
      target_type: data.target_type as string,
      target_id: data.target_id as string,
      user_id: data.user_id as string,
      reaction_type: data.reaction_type as string,
    });

    logger.info('Reaction added', { id: reaction.id });
  }

  private async handleReactionRemoved(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling reaction.removed', { data });

    const deleted = await this.db.removeReaction(
      data.target_type as string,
      data.target_id as string,
      data.user_id as string,
      data.reaction_type as string
    );

    if (deleted) {
      logger.info('Reaction removed');
    } else {
      logger.warn('Reaction not found for removal');
    }
  }

  private async handleFollowCreated(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling follow.created', { data });

    const follow = await this.db.createFollow({
      follower_id: data.follower_id as string,
      following_type: data.following_type as never,
      following_id: data.following_id as string,
    });

    logger.info('Follow created', { id: follow.id });
  }

  private async handleFollowDeleted(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling follow.deleted', { data });

    const deleted = await this.db.deleteFollow(
      data.follower_id as string,
      data.following_type as string,
      data.following_id as string
    );

    if (deleted) {
      logger.info('Follow deleted');
    } else {
      logger.warn('Follow not found for deletion');
    }
  }

  private async handleBookmarkCreated(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling bookmark.created', { data });

    const bookmark = await this.db.createBookmark({
      user_id: data.user_id as string,
      target_type: data.target_type as string,
      target_id: data.target_id as string,
      collection: data.collection as string,
      note: data.note as string,
    });

    logger.info('Bookmark created', { id: bookmark.id });
  }

  private async handleBookmarkDeleted(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling bookmark.deleted', { data });

    const deleted = await this.db.deleteBookmark(
      data.user_id as string,
      data.target_type as string,
      data.target_id as string
    );

    if (deleted) {
      logger.info('Bookmark deleted');
    } else {
      logger.warn('Bookmark not found for deletion');
    }
  }

  private async handleShareCreated(data: Record<string, unknown>): Promise<void> {
    logger.debug('Handling share.created', { data });

    const share = await this.db.createShare({
      user_id: data.user_id as string,
      target_type: data.target_type as string,
      target_id: data.target_id as string,
      share_type: data.share_type as never,
      message: data.message as string,
    });

    logger.info('Share created', { id: share.id });
  }
}
