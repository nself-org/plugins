package internal

import "time"

type Wallet struct {
	ID                     string     `json:"id"`
	SourceAccountID        string     `json:"source_account_id"`
	UserID                 string     `json:"user_id"`
	WorkspaceID            *string    `json:"workspace_id,omitempty"`
	Address                string     `json:"address"`
	ChainID                int        `json:"chain_id"`
	ChainName              string     `json:"chain_name"`
	WalletType             *string    `json:"wallet_type,omitempty"`
	EnsName                *string    `json:"ens_name,omitempty"`
	EnsAvatar              *string    `json:"ens_avatar,omitempty"`
	Label                  *string    `json:"label,omitempty"`
	IsPrimary              bool       `json:"is_primary"`
	VerifiedAt             *time.Time `json:"verified_at,omitempty"`
	VerificationSignature  *string    `json:"verification_signature,omitempty"`
	VerificationMessage    *string    `json:"verification_message,omitempty"`
	IsActive               bool       `json:"is_active"`
	Metadata               []byte     `json:"metadata,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type Collection struct {
	ID                 string     `json:"id"`
	SourceAccountID    string     `json:"source_account_id"`
	ContractAddress    string     `json:"contract_address"`
	ChainID            int        `json:"chain_id"`
	Name               string     `json:"name"`
	Slug               *string    `json:"slug,omitempty"`
	Description        *string    `json:"description,omitempty"`
	ImageURL           *string    `json:"image_url,omitempty"`
	BannerURL          *string    `json:"banner_url,omitempty"`
	FeaturedImageURL   *string    `json:"featured_image_url,omitempty"`
	WebsiteURL         *string    `json:"website_url,omitempty"`
	TwitterUsername    *string    `json:"twitter_username,omitempty"`
	DiscordURL         *string    `json:"discord_url,omitempty"`
	TelegramURL        *string    `json:"telegram_url,omitempty"`
	TokenStandard      *string    `json:"token_standard,omitempty"`
	TotalSupply        *int64     `json:"total_supply,omitempty"`
	FloorPrice         *string    `json:"floor_price,omitempty"`
	FloorPriceCurrency *string    `json:"floor_price_currency,omitempty"`
	VolumeTotal        *string    `json:"volume_total,omitempty"`
	Volume24h          *string    `json:"volume_24h,omitempty"`
	OwnersCount        *int       `json:"owners_count,omitempty"`
	IsVerified         bool       `json:"is_verified"`
	IsSpam             bool       `json:"is_spam"`
	WorkspaceID        *string    `json:"workspace_id,omitempty"`
	IsManaged          bool       `json:"is_managed"`
	Metadata           []byte     `json:"metadata,omitempty"`
	LastIndexedAt      *time.Time `json:"last_indexed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type NFT struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ContractAddress string     `json:"contract_address"`
	TokenID         string     `json:"token_id"`
	ChainID         int        `json:"chain_id"`
	TokenStandard   string     `json:"token_standard"`
	OwnerAddress    string     `json:"owner_address"`
	OwnerUserID     *string    `json:"owner_user_id,omitempty"`
	Quantity        int64      `json:"quantity"`
	Name            *string    `json:"name,omitempty"`
	Description     *string    `json:"description,omitempty"`
	ImageURL        *string    `json:"image_url,omitempty"`
	AnimationURL    *string    `json:"animation_url,omitempty"`
	ExternalURL     *string    `json:"external_url,omitempty"`
	Attributes      []byte     `json:"attributes,omitempty"`
	MetadataURI     *string    `json:"metadata_uri,omitempty"`
	Metadata        []byte     `json:"metadata,omitempty"`
	CollectionID    *string    `json:"collection_id,omitempty"`
	RarityScore     *string    `json:"rarity_score,omitempty"`
	RarityRank      *int       `json:"rarity_rank,omitempty"`
	IsVerified      bool       `json:"is_verified"`
	IsSpam          bool       `json:"is_spam"`
	LastIndexedAt   *time.Time `json:"last_indexed_at,omitempty"`
	BlockNumber     *int64     `json:"block_number,omitempty"`
	MintedAt        *time.Time `json:"minted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Token struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ContractAddress string     `json:"contract_address"`
	ChainID         int        `json:"chain_id"`
	Name            string     `json:"name"`
	Symbol          string     `json:"symbol"`
	Decimals        int        `json:"decimals"`
	TokenType       string     `json:"token_type"`
	PriceUSD        *string    `json:"price_usd,omitempty"`
	PriceUpdatedAt  *time.Time `json:"price_updated_at,omitempty"`
	LogoURL         *string    `json:"logo_url,omitempty"`
	WebsiteURL      *string    `json:"website_url,omitempty"`
	Description     *string    `json:"description,omitempty"`
	IsVerified      bool       `json:"is_verified"`
	IsSpam          bool       `json:"is_spam"`
	Metadata        []byte     `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type TokenBalance struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	WalletAddress   string     `json:"wallet_address"`
	UserID          *string    `json:"user_id,omitempty"`
	TokenID         string     `json:"token_id"`
	Balance         string     `json:"balance"`
	BalanceFormatted *string   `json:"balance_formatted,omitempty"`
	ValueUSD        *string    `json:"value_usd,omitempty"`
	LastUpdatedAt   time.Time  `json:"last_updated_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type TokenGate struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	WorkspaceID     string    `json:"workspace_id"`
	CreatedBy       string    `json:"created_by"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	GateType        string    `json:"gate_type"`
	Rules           []byte    `json:"rules"`
	TargetType      string    `json:"target_type"`
	TargetID        *string   `json:"target_id,omitempty"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type GateCheck struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	GateID          string     `json:"gate_id"`
	UserID          string     `json:"user_id"`
	WalletAddress   string     `json:"wallet_address"`
	Passed          bool       `json:"passed"`
	FailureReason   *string    `json:"failure_reason,omitempty"`
	Evidence        []byte     `json:"evidence,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CheckedAt       time.Time  `json:"checked_at"`
}

type DAO struct {
	ID                      string     `json:"id"`
	SourceAccountID         string     `json:"source_account_id"`
	WorkspaceID             *string    `json:"workspace_id,omitempty"`
	Name                    string     `json:"name"`
	Slug                    *string    `json:"slug,omitempty"`
	Description             *string    `json:"description,omitempty"`
	ChainID                 int        `json:"chain_id"`
	GovernanceTokenAddress  *string    `json:"governance_token_address,omitempty"`
	TreasuryAddress         *string    `json:"treasury_address,omitempty"`
	SnapshotSpace           *string    `json:"snapshot_space,omitempty"`
	GovernorAddress         *string    `json:"governor_address,omitempty"`
	GovernorType            *string    `json:"governor_type,omitempty"`
	ProposalThreshold       *string    `json:"proposal_threshold,omitempty"`
	Quorum                  *string    `json:"quorum,omitempty"`
	VotingDelay             *int64     `json:"voting_delay,omitempty"`
	VotingPeriod            *int64     `json:"voting_period,omitempty"`
	IsActive                bool       `json:"is_active"`
	Metadata                []byte     `json:"metadata,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type Proposal struct {
	ID                   string     `json:"id"`
	SourceAccountID      string     `json:"source_account_id"`
	DaoID                string     `json:"dao_id"`
	Title                string     `json:"title"`
	Description          string     `json:"description"`
	ProposerAddress      string     `json:"proposer_address"`
	ProposerUserID       *string    `json:"proposer_user_id,omitempty"`
	ChainProposalID      *string    `json:"chain_proposal_id,omitempty"`
	SnapshotProposalID   *string    `json:"snapshot_proposal_id,omitempty"`
	Status               string     `json:"status"`
	StartBlock           *int64     `json:"start_block,omitempty"`
	EndBlock             *int64     `json:"end_block,omitempty"`
	StartTime            *time.Time `json:"start_time,omitempty"`
	EndTime              *time.Time `json:"end_time,omitempty"`
	VotesFor             string     `json:"votes_for"`
	VotesAgainst         string     `json:"votes_against"`
	VotesAbstain         string     `json:"votes_abstain"`
	Executable           bool       `json:"executable"`
	ExecutedAt           *time.Time `json:"executed_at,omitempty"`
	ExecutionTxHash      *string    `json:"execution_tx_hash,omitempty"`
	Metadata             []byte     `json:"metadata,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Vote struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ProposalID      string     `json:"proposal_id"`
	VoterAddress    string     `json:"voter_address"`
	VoterUserID     *string    `json:"voter_user_id,omitempty"`
	Support         string     `json:"support"`
	VotingPower     string     `json:"voting_power"`
	Reason          *string    `json:"reason,omitempty"`
	TransactionHash *string    `json:"transaction_hash,omitempty"`
	BlockNumber     *int64     `json:"block_number,omitempty"`
	VotedAt         time.Time  `json:"voted_at"`
}

type Transaction struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	TransactionHash string     `json:"transaction_hash"`
	ChainID         int        `json:"chain_id"`
	FromAddress     string     `json:"from_address"`
	ToAddress       *string    `json:"to_address,omitempty"`
	FromUserID      *string    `json:"from_user_id,omitempty"`
	ToUserID        *string    `json:"to_user_id,omitempty"`
	Value           string     `json:"value"`
	ValueUSD        *string    `json:"value_usd,omitempty"`
	GasUsed         *int64     `json:"gas_used,omitempty"`
	GasPrice        *string    `json:"gas_price,omitempty"`
	BlockNumber     *int64     `json:"block_number,omitempty"`
	BlockTimestamp  *time.Time `json:"block_timestamp,omitempty"`
	Status          *string    `json:"status,omitempty"`
	TransactionType *string    `json:"transaction_type,omitempty"`
	InputData       *string    `json:"input_data,omitempty"`
	Metadata        []byte     `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Web3Event struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	EventName       string     `json:"event_name"`
	ContractAddress string     `json:"contract_address"`
	ChainID         int        `json:"chain_id"`
	TransactionHash string     `json:"transaction_hash"`
	LogIndex        int        `json:"log_index"`
	BlockNumber     int64      `json:"block_number"`
	BlockTimestamp  *time.Time `json:"block_timestamp,omitempty"`
	EventData       []byte     `json:"event_data"`
	DecodedData     []byte     `json:"decoded_data,omitempty"`
	RelatedNftID    *string    `json:"related_nft_id,omitempty"`
	RelatedUserID   *string    `json:"related_user_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}
