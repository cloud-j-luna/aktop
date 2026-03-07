package consensus

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ConsensusResponse represents the JSON-RPC response from /consensus_state
type ConsensusResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		RoundState RoundState `json:"round_state"`
	} `json:"result"`
}

// RoundState contains the consensus round information
type RoundState struct {
	HeightRoundStep   string       `json:"height/round/step"`
	StartTime         time.Time    `json:"start_time"`
	ProposalBlockHash string       `json:"proposal_block_hash"`
	LockedBlockHash   string       `json:"locked_block_hash"`
	ValidBlockHash    string       `json:"valid_block_hash"`
	HeightVoteSet     []HeightVote `json:"height_vote_set"`
	Proposer          ProposerInfo `json:"proposer"`
}

// HeightVote contains vote information for a round
type HeightVote struct {
	Round              int      `json:"round"`
	Prevotes           []string `json:"prevotes"`
	PrevotesBitArray   string   `json:"prevotes_bit_array"`
	Precommits         []string `json:"precommits"`
	PrecommitsBitArray string   `json:"precommits_bit_array"`
}

// ProposerInfo contains proposer details
type ProposerInfo struct {
	Address string `json:"address"`
	Index   int    `json:"index"`
}

// ValidatorsResponse represents the JSON-RPC response from /validators
type ValidatorsResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		BlockHeight string      `json:"block_height"`
		Validators  []Validator `json:"validators"`
		Count       string      `json:"count"`
		Total       string      `json:"total"`
	} `json:"result"`
}

// Validator represents a validator in the set
type Validator struct {
	Address          string `json:"address"`
	PubKey           PubKey `json:"pub_key"`
	VotingPower      string `json:"voting_power"`
	ProposerPriority string `json:"proposer_priority"`
}

// PubKey represents a validator's public key
type PubKey struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ValidatorStatus represents a validator's current voting status
type ValidatorStatus struct {
	Index       int
	Address     string
	PubKey      string // Base64 encoded consensus pubkey for moniker lookup
	VotingPower int64
	Prevoted    bool
	Precommited bool
	IsProposer  bool
}

// State represents the parsed consensus state for display
type State struct {
	Height          int64
	Round           int
	Step            string
	StartTime       time.Time
	Elapsed         time.Duration
	ProposerAddress string
	ProposerIndex   int

	// Vote stats
	TotalValidators  int
	TotalVotingPower int64

	// Prevote stats
	PrevoteCount    int
	PrevotePower    int64
	PrevotePercent  float64
	PrevoteBitArray string

	// Precommit stats
	PrecommitCount    int
	PrecommitPower    int64
	PrecommitPercent  float64
	PrecommitBitArray string

	// Validator list with vote status
	Validators []ValidatorStatus

	// Error state
	Error error
}

// ParseHeightRoundStep parses the "height/round/step" string
func ParseHeightRoundStep(hrs string) (height int64, round int, step string, err error) {
	parts := strings.Split(hrs, "/")
	if len(parts) != 3 {
		return 0, 0, "", fmt.Errorf("invalid height/round/step format: %s", hrs)
	}

	height, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("invalid height: %v", err)
	}

	round, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, "", fmt.Errorf("invalid round: %v", err)
	}

	stepNum, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, "", fmt.Errorf("invalid step: %v", err)
	}

	// Map step number to name
	stepNames := map[int]string{
		1: "NewHeight",
		2: "NewRound",
		3: "Propose",
		4: "Prevote",
		5: "PrevoteWait",
		6: "Precommit",
		7: "PrecommitWait",
		8: "Commit",
	}

	step = stepNames[stepNum]
	if step == "" {
		step = fmt.Sprintf("Unknown(%d)", stepNum)
	}

	return height, round, step, nil
}

// ParseBitArray extracts the bit pattern and percentages from the bit array string
// Format: "BA{99:xx_xx_x_...} 63976000/106137430 = 0.60"
func ParseBitArray(bitArray string) (pattern string, voted int64, total int64, percent float64) {
	// Extract pattern between { and }
	patternRegex := regexp.MustCompile(`BA\{\d+:([x_]+)\}`)
	if matches := patternRegex.FindStringSubmatch(bitArray); len(matches) > 1 {
		pattern = matches[1]
	}

	// Extract voted/total
	statsRegex := regexp.MustCompile(`(\d+)/(\d+)\s*=\s*([\d.]+)`)
	if matches := statsRegex.FindStringSubmatch(bitArray); len(matches) > 3 {
		voted, _ = strconv.ParseInt(matches[1], 10, 64)
		total, _ = strconv.ParseInt(matches[2], 10, 64)
		percent, _ = strconv.ParseFloat(matches[3], 64)
	}

	return pattern, voted, total, percent
}

// CountVotes counts the number of non-nil votes in a vote array
func CountVotes(votes []string) int {
	count := 0
	for _, v := range votes {
		if v != "nil-Vote" {
			count++
		}
	}
	return count
}

// ParseConsensusState converts raw consensus data into a State struct
func ParseConsensusState(resp *ConsensusResponse, validators []Validator) (*State, error) {
	rs := resp.Result.RoundState

	height, round, step, err := ParseHeightRoundStep(rs.HeightRoundStep)
	if err != nil {
		return nil, err
	}

	state := &State{
		Height:          height,
		Round:           round,
		Step:            step,
		StartTime:       rs.StartTime,
		Elapsed:         time.Since(rs.StartTime),
		ProposerAddress: rs.Proposer.Address,
		ProposerIndex:   rs.Proposer.Index,
		TotalValidators: len(validators),
	}

	// Parse vote data for current round
	if len(rs.HeightVoteSet) > round {
		voteSet := rs.HeightVoteSet[round]

		// Prevotes
		state.PrevoteCount = CountVotes(voteSet.Prevotes)
		state.PrevoteBitArray, state.PrevotePower, state.TotalVotingPower, state.PrevotePercent =
			ParseBitArray(voteSet.PrevotesBitArray)

		// Precommits
		state.PrecommitCount = CountVotes(voteSet.Precommits)
		state.PrecommitBitArray, state.PrecommitPower, _, state.PrecommitPercent =
			ParseBitArray(voteSet.PrecommitsBitArray)

		if len(voteSet.Prevotes) > 0 {
			state.TotalValidators = len(voteSet.Prevotes)
		}

		// Build validator status list with vote data
		state.Validators = buildValidatorStatus(validators, voteSet, rs.Proposer.Index)
	} else {
		// No vote data yet, but still build validator list
		state.Validators = buildValidatorStatusNoVotes(validators, rs.Proposer.Index)
	}

	return state, nil
}

// buildValidatorStatusNoVotes creates a validator list when no vote data is available
func buildValidatorStatusNoVotes(validators []Validator, proposerIndex int) []ValidatorStatus {
	result := make([]ValidatorStatus, len(validators))
	for i, v := range validators {
		power, _ := strconv.ParseInt(v.VotingPower, 10, 64)
		result[i] = ValidatorStatus{
			Index:       i,
			Address:     v.Address,
			PubKey:      v.PubKey.Value,
			VotingPower: power,
			Prevoted:    false,
			Precommited: false,
			IsProposer:  i == proposerIndex,
		}
	}
	return result
}

// buildValidatorStatus creates a list of validators with their vote status
func buildValidatorStatus(validators []Validator, voteSet HeightVote, proposerIndex int) []ValidatorStatus {
	// Create a map of prevotes and precommits by index
	prevoteMap := make(map[int]bool)
	precommitMap := make(map[int]bool)

	for i, vote := range voteSet.Prevotes {
		if vote != "nil-Vote" {
			prevoteMap[i] = true
		}
	}

	for i, vote := range voteSet.Precommits {
		if vote != "nil-Vote" {
			precommitMap[i] = true
		}
	}

	// Build status list
	result := make([]ValidatorStatus, len(validators))
	for i, v := range validators {
		power, _ := strconv.ParseInt(v.VotingPower, 10, 64)
		result[i] = ValidatorStatus{
			Index:       i,
			Address:     v.Address,
			PubKey:      v.PubKey.Value, // Base64 encoded pubkey for moniker lookup
			VotingPower: power,
			Prevoted:    prevoteMap[i],
			Precommited: precommitMap[i],
			IsProposer:  i == proposerIndex,
		}
	}

	return result
}
