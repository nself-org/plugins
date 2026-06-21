package internal

import (
	"context"
	"fmt"
)

func (s *SyncService) syncCommits(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	commits, err := s.client.ListCommits(owner, repo)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("commits %s/%s: %v", owner, repo, err))
		return
	}

	for _, c := range commits {
		var authorLogin, committerLogin *string
		if c.Author != nil {
			authorLogin = &c.Author.Login
		}
		if c.Committer != nil {
			committerLogin = &c.Committer.Login
		}

		msg := c.Commit.Message
		authorName := c.Commit.Author.Name
		authorEmail := c.Commit.Author.Email
		committerName := c.Commit.Committer.Name
		committerEmail := c.Commit.Committer.Email
		treeSHA := c.Commit.Tree.SHA
		parents := &c.Parents
		htmlURL := c.HTMLURL
		reason := c.Commit.Verification.Reason

		var additions, deletions, total int
		if c.Stats != nil {
			additions = c.Stats.Additions
			deletions = c.Stats.Deletions
			total = c.Stats.Total
		}

		commit := &Commit{
			SHA:                c.SHA,
			SourceAccountID:    s.acctID,
			NodeID:             &c.NodeID,
			RepoID:             &repoID,
			Message:            &msg,
			AuthorName:         &authorName,
			AuthorEmail:        &authorEmail,
			AuthorLogin:        authorLogin,
			AuthorDate:         c.Commit.Author.Date,
			CommitterName:      &committerName,
			CommitterEmail:     &committerEmail,
			CommitterLogin:     committerLogin,
			CommitterDate:      c.Commit.Committer.Date,
			TreeSHA:            &treeSHA,
			Parents:            parents,
			CommitAdditions:    additions,
			CommitDeletions:    deletions,
			Total:              total,
			HTMLURL:            &htmlURL,
			Verified:           c.Commit.Verification.Verified,
			VerificationReason: &reason,
		}
		if err := UpsertCommit(ctx, s.pool, commit); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("commit %s/%s:%s: %v", owner, repo, c.SHA[:7], err))
		}
	}
	result.Stats.Commits += len(commits)
}
