package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	solana "github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	tokenprog "github.com/gagliardetto/solana-go/programs/token"
)

const (
	base58Alphabet              = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	systemProgramAddress        = "11111111111111111111111111111111"
	computeBudgetProgramAddress = "ComputeBudget111111111111111111111111111111"
)

var (
	bigRadix = big.NewInt(58)
	bigZero  = big.NewInt(0)
	indexes  = buildAlphabetIndexes()
)

func buildAlphabetIndexes() map[rune]int {
	out := make(map[rune]int, len(base58Alphabet))
	for i, ch := range base58Alphabet {
		out[ch] = i
	}
	return out
}

func decodeBase58(input string) ([]byte, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("invalid base58 payload")
	}
	result := big.NewInt(0)
	for _, ch := range input {
		idx, ok := indexes[ch]
		if !ok {
			return nil, fmt.Errorf("invalid base58 payload")
		}
		result.Mul(result, bigRadix)
		result.Add(result, big.NewInt(int64(idx)))
	}
	decoded := result.Bytes()
	leadingZeros := 0
	for _, ch := range input {
		if ch != rune(base58Alphabet[0]) {
			break
		}
		leadingZeros++
	}
	if leadingZeros == 0 {
		return decoded, nil
	}
	output := make([]byte, leadingZeros+len(decoded))
	copy(output[leadingZeros:], decoded)
	return output, nil
}

type message struct {
	NumRequiredSignatures byte
	NumReadonlySigned     byte
	NumReadonlyUnsigned   byte
	AccountKeys           [][]byte
	RecentBlockhash       []byte
	Instructions          []compiledInstruction
}

type compiledInstruction struct {
	ProgramIndex byte
	AccountIdxs  []byte
	Data         []byte
}

func buildUnsignedNativeTransferTx(fromAddress string, toAddress string, recentBlockhash string, lamports uint64, computeUnitPrice uint64) (string, error) {
	fromBytes, err := decodeBase58(fromAddress)
	if err != nil || len(fromBytes) != 32 {
		return "", fmt.Errorf("decode from address: %w", err)
	}
	toBytes, err := decodeBase58(toAddress)
	if err != nil || len(toBytes) != 32 {
		return "", fmt.Errorf("decode to address: %w", err)
	}
	systemProgramBytes, _ := decodeBase58(systemProgramAddress)

	accountKeys := [][]byte{fromBytes, toBytes, systemProgramBytes}
	instructions := make([]compiledInstruction, 0, 2)
	readonlyUnsigned := byte(1)

	if computeUnitPrice > 0 {
		computeBudgetBytes, _ := decodeBase58(computeBudgetProgramAddress)
		accountKeys = append(accountKeys, computeBudgetBytes)
		readonlyUnsigned = 2

		data := make([]byte, 9)
		data[0] = 3
		binary.LittleEndian.PutUint64(data[1:], computeUnitPrice)
		instructions = append(instructions, compiledInstruction{
			ProgramIndex: 3,
			AccountIdxs:  []byte{},
			Data:         data,
		})
	}

	transferData := make([]byte, 12)
	binary.LittleEndian.PutUint32(transferData[:4], 2)
	binary.LittleEndian.PutUint64(transferData[4:], lamports)
	instructions = append(instructions, compiledInstruction{
		ProgramIndex: 2,
		AccountIdxs:  []byte{0, 1},
		Data:         transferData,
	})

	blockhashBytes, err := decodeBase58(recentBlockhash)
	if err != nil || len(blockhashBytes) != 32 {
		return "", fmt.Errorf("decode recent blockhash: %w", err)
	}

	msg := encodeMessage(message{
		NumRequiredSignatures: 1,
		NumReadonlySigned:     0,
		NumReadonlyUnsigned:   readonlyUnsigned,
		AccountKeys:           accountKeys,
		RecentBlockhash:       blockhashBytes,
		Instructions:          instructions,
	})

	txBytes := encodeCompactU16(1)
	txBytes = append(txBytes, make([]byte, 64)...)
	txBytes = append(txBytes, msg...)
	return base64.StdEncoding.EncodeToString(txBytes), nil
}

func encodeMessage(msg message) []byte {
	out := []byte{msg.NumRequiredSignatures, msg.NumReadonlySigned, msg.NumReadonlyUnsigned}
	out = append(out, encodeCompactU16(len(msg.AccountKeys))...)
	for _, key := range msg.AccountKeys {
		out = append(out, key...)
	}
	out = append(out, msg.RecentBlockhash...)
	out = append(out, encodeCompactU16(len(msg.Instructions))...)
	for _, ix := range msg.Instructions {
		out = append(out, ix.ProgramIndex)
		out = append(out, encodeCompactU16(len(ix.AccountIdxs))...)
		out = append(out, ix.AccountIdxs...)
		out = append(out, encodeCompactU16(len(ix.Data))...)
		out = append(out, ix.Data...)
	}
	return out
}

func encodeCompactU16(v int) []byte {
	if v < 0 {
		return []byte{0}
	}
	out := make([]byte, 0, 3)
	value := uint32(v)
	for {
		elem := byte(value & 0x7f)
		value >>= 7
		if value == 0 {
			out = append(out, elem)
			return out
		}
		out = append(out, elem|0x80)
	}
}

func validateSolanaAddress(address string) bool {
	raw, err := decodeBase58(address)
	return err == nil && len(raw) == 32
}

func amountToLamports(amount string) (uint64, error) {
	value, err := parseAmount(amount)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("amount must be greater than 0")
	}
	lamports := value * 1_000_000_000
	if lamports > math.MaxUint64 {
		return 0, fmt.Errorf("amount too large")
	}
	return uint64(math.Round(lamports)), nil
}

func fetchLatestBlockhash(ctx context.Context, httpClient *http.Client, rpcEndpoint string) (string, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getLatestBlockhash",
		"params":  []interface{}{map[string]interface{}{"commitment": "confirmed"}},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Error  interface{} `json:"error"`
		Result struct {
			Value struct {
				Blockhash string `json:"blockhash"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("getLatestBlockhash failed")
	}
	if result.Result.Value.Blockhash == "" {
		return "", fmt.Errorf("empty latest blockhash")
	}
	return result.Result.Value.Blockhash, nil
}

type tokenAccountInfo struct {
	Pubkey   string
	Mint     string
	Amount   string
	Decimals uint8
}

func fetchTokenAccountsByOwner(ctx context.Context, httpClient *http.Client, rpcEndpoint string, ownerAddress string, mintAddress string) ([]tokenAccountInfo, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTokenAccountsByOwner",
		"params": []interface{}{
			ownerAddress,
			map[string]interface{}{"mint": mintAddress},
			map[string]interface{}{
				"encoding":   "jsonParsed",
				"commitment": "confirmed",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Error  interface{} `json:"error"`
		Result struct {
			Value []struct {
				Pubkey  string `json:"pubkey"`
				Account struct {
					Data struct {
						Parsed struct {
							Info struct {
								Mint        string `json:"mint"`
								TokenAmount struct {
									Amount   string `json:"amount"`
									Decimals uint8  `json:"decimals"`
								} `json:"tokenAmount"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("getTokenAccountsByOwner failed")
	}
	items := make([]tokenAccountInfo, 0, len(result.Result.Value))
	for _, item := range result.Result.Value {
		items = append(items, tokenAccountInfo{
			Pubkey:   item.Pubkey,
			Mint:     item.Account.Data.Parsed.Info.Mint,
			Amount:   item.Account.Data.Parsed.Info.TokenAmount.Amount,
			Decimals: item.Account.Data.Parsed.Info.TokenAmount.Decimals,
		})
	}
	return items, nil
}

func buildUnsignedSPLTransferTx(fromOwnerAddress string, toOwnerAddress string, mintAddress string, sourceTokenAccount string, destinationTokenAccount string, recentBlockhash string, amount uint64, decimals uint8, computeUnitPrice uint64, createATA bool) (string, error) {
	fromOwner, err := solana.PublicKeyFromBase58(fromOwnerAddress)
	if err != nil {
		return "", err
	}
	toOwner, err := solana.PublicKeyFromBase58(toOwnerAddress)
	if err != nil {
		return "", err
	}
	mint, err := solana.PublicKeyFromBase58(mintAddress)
	if err != nil {
		return "", err
	}
	sourceATA, err := solana.PublicKeyFromBase58(sourceTokenAccount)
	if err != nil {
		return "", err
	}
	destATA, err := solana.PublicKeyFromBase58(destinationTokenAccount)
	if err != nil {
		return "", err
	}
	blockhash, err := solana.HashFromBase58(recentBlockhash)
	if err != nil {
		return "", err
	}

	instructions := make([]solana.Instruction, 0, 3)
	if computeUnitPrice > 0 {
		ix, err := computebudget.NewSetComputeUnitPriceInstruction(computeUnitPrice).ValidateAndBuild()
		if err != nil {
			return "", err
		}
		instructions = append(instructions, ix)
	}
	if createATA {
		ix, err := associatedtokenaccount.NewCreateInstruction(fromOwner, toOwner, mint).ValidateAndBuild()
		if err != nil {
			return "", err
		}
		instructions = append(instructions, ix)
	}
	ix, err := tokenprog.NewTransferCheckedInstruction(
		amount,
		decimals,
		sourceATA,
		mint,
		destATA,
		fromOwner,
		nil,
	).ValidateAndBuild()
	if err != nil {
		return "", err
	}
	instructions = append(instructions, ix)

	tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(fromOwner))
	if err != nil {
		return "", err
	}
	return tx.ToBase64()
}

func deriveAssociatedTokenAddress(ownerAddress string, mintAddress string) (string, bool, error) {
	owner, err := solana.PublicKeyFromBase58(ownerAddress)
	if err != nil {
		return "", false, err
	}
	mint, err := solana.PublicKeyFromBase58(mintAddress)
	if err != nil {
		return "", false, err
	}
	ata, _, err := solana.FindAssociatedTokenAddress(owner, mint)
	if err != nil {
		return "", false, err
	}
	return ata.String(), true, nil
}

func amountToTokenUnits(amount string, decimals uint8) (uint64, error) {
	trimmed := strings.TrimSpace(amount)
	if trimmed == "" {
		return 0, fmt.Errorf("invalid amount")
	}
	if strings.HasPrefix(trimmed, "-") {
		return 0, fmt.Errorf("amount must be greater than 0")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount")
	}
	intPart := parts[0]
	if intPart == "" {
		intPart = "0"
	}
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if len(fracPart) > int(decimals) {
		return 0, fmt.Errorf("amount exceeds token decimals")
	}
	for len(fracPart) < int(decimals) {
		fracPart += "0"
	}
	base := intPart + fracPart
	base = strings.TrimLeft(base, "0")
	if base == "" {
		base = "0"
	}
	value, err := strconv.ParseUint(base, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount")
	}
	if value == 0 {
		return 0, fmt.Errorf("amount must be greater than 0")
	}
	return value, nil
}
