package codec

import (
	"bufio"
	"fmt"
	"io"
	// "math/big"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	pbcodec "github.com/ChainSafe/firehose-arweave/pb/cs/arweave/codec/v1"
	"github.com/golang/protobuf/proto"
	"go.uber.org/zap"
)

// ConsoleReader is what reads the `geth` output directly. It builds
// up some LogEntry objects. See `LogReader to read those entries .
type ConsoleReader struct {
	lines chan string
	close func()

	ctx  *parseCtx
	done chan interface{}
}

func NewConsoleReader(lines chan string, rpcUrl string) (*ConsoleReader, error) {
	l := &ConsoleReader{
		lines: lines,
		close: func() {},
		done:  make(chan interface{}),
	}
	return l, nil
}

//todo: WTF?
func (r *ConsoleReader) Done() <-chan interface{} {
	return r.done
}

func (r *ConsoleReader) Close() {
	r.close()
}

type parsingStats struct {
	startAt  time.Time
	blockNum uint64
	data     map[string]int
}

func newParsingStats(block uint64) *parsingStats {
	return &parsingStats{
		startAt:  time.Now(),
		blockNum: block,
		data:     map[string]int{},
	}
}

func (s *parsingStats) log() {
	zlog.Info("mindreader block stats",
		zap.Uint64("block_num", s.blockNum),
		zap.Int64("duration", int64(time.Since(s.startAt))),
		zap.Reflect("stats", s.data),
	)
}

func (s *parsingStats) inc(key string) {
	if s == nil {
		return
	}
	k := strings.ToLower(key)
	value := s.data[k]
	value++
	s.data[k] = value
}

type parseCtx struct {
	currentBlock *pbcodec.Block

	stats *parsingStats
}

func newContext(height uint64) *parseCtx {
	return &parseCtx{
		currentBlock: &pbcodec.Block{
			Height: height,
			Txs:    []*pbcodec.Transaction{},
		},
	}

}

func (r *ConsoleReader) Read() (out interface{}, err error) {
	return r.next()
}

const (
	LogPrefix = "DMLOG"
	LogBlock  = "BLOCK"
)

func (r *ConsoleReader) next() (out interface{}, err error) {
	for line := range r.lines {
		if !strings.HasPrefix(line, LogPrefix) {
			continue
		}

		tokens := strings.Split(line[len(LogPrefix)+1:], " ")
		if len(tokens) < 2 {
			return nil, fmt.Errorf("invalid log line format: %s", line)
		}

		switch tokens[0] {
		case LogBlock:
			return r.block(tokens[1:])
		default:
			if tracer.Enabled() {
				zlog.Debug("skipping unknown deep mind log line", zap.String("line", line))
			}
			continue
		}

		if err != nil {
			chunks := strings.SplitN(line, " ", 2)
			return nil, fmt.Errorf("%s: %s (line %q)", chunks[0], err, line)
		}
	}

	zlog.Info("lines channel has been closed")
	return nil, io.EOF
}

func (r *ConsoleReader) ProcessData(reader io.Reader) error {
	scanner := r.buildScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		r.lines <- line
	}

	if scanner.Err() == nil {
		close(r.lines)
		return io.EOF
	}

	return scanner.Err()
}

func (r *ConsoleReader) buildScanner(reader io.Reader) *bufio.Scanner {
	buf := make([]byte, 50*1024*1024)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(buf, 50*1024*1024)

	return scanner
}

// Format:
// DMLOG BLOCK <HEIGHT> <ENCODED_BLOCK>
func (r *ConsoleReader) block(params []string) (*pbcodec.Block, error) {
	if err := validateChunk(params, 2); err != nil {
		return nil, fmt.Errorf("invalid log line length: %w", err)
	}

	// <HEIGHT>
	//
	// parse block height
	blockHeight, err := strconv.ParseUint(params[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid block num: %w", err)
	}

	// <ENCODED_BLOCK>
	//
	// hex decode block
	bytes, err := hex.DecodeString(params[1])
	if err != nil {
		return nil, fmt.Errorf("invalid encoded block: %w", err)
	}

	// decode bytes to Block
	if proto.Unmarshal(bytes, r.ctx.currentBlock) != nil {
		return nil, fmt.Errorf("invalid encoded block: %w", err)
	}

	return r.ctx.currentBlock, nil
}

// // Format:
// // DMLOG BLOCK_BEGIN <HASH> <TYPE> <FROM> <TO> <AMOUNT> <FEE> <SUCCESS>
// func (ctx *parseCtx) trxBegin(params []string) error {
// 	if err := validateChunk(params, 7); err != nil {
// 		return fmt.Errorf("invalid log line length: %w", err)
// 	}
// 	if ctx == nil {
// 		return fmt.Errorf("did not process a BLOCK_BEGIN")
// 	}
//
// 	trx := &pbcodec.Transaction{
// 		Type:     params[1],
// 		Hash:     params[0],
// 		Sender:   params[2],
// 		Receiver: params[3],
// 		Success:  params[6] == "true",
// 		Events:   []*pbcodec.Event{},
// 	}
//
// 	v, ok := new(big.Int).SetString(params[4], 16)
// 	if !ok {
// 		return fmt.Errorf("unable to parse trx amount %s", params[4])
// 	}
// 	trx.Amount = &pbcodec.BigInt{Bytes: v.Bytes()}
//
// 	v, ok = new(big.Int).SetString(params[5], 16)
// 	if !ok {
// 		return fmt.Errorf("unable to parse trx amount %s", params[4])
// 	}
// 	trx.Fee = &pbcodec.BigInt{Bytes: v.Bytes()}
//
// 	ctx.currentBlock.Transactions = append(ctx.currentBlock.Transactions, trx)
// 	return nil
// }
//
// // Format:
// // DMLOG TRX_BEGIN_EVENT <TRX_HASH> <TYPE>
//
// func (ctx *parseCtx) eventBegin(params []string) error {
// 	if err := validateChunk(params, 2); err != nil {
// 		return fmt.Errorf("invalid log line length: %w", err)
// 	}
// 	if ctx == nil {
// 		return fmt.Errorf("did not process a BLOCK_BEGIN")
// 	}
// 	if len(ctx.currentBlock.Transactions) == 0 {
// 		return fmt.Errorf("did not process a BEGIN_TRX")
// 	}
//
// 	trx := ctx.currentBlock.Transactions[len(ctx.currentBlock.Transactions)-1]
// 	if trx.Hash != params[0] {
// 		return fmt.Errorf("last transaction hash %q does not match the event trx hash %q", trx.Hash, params[0])
// 	}
//
// 	trx.Events = append(trx.Events, &pbcodec.Event{
// 		Type:       params[1],
// 		Attributes: []*pbcodec.Attribute{},
// 	})
//
// 	ctx.currentBlock.Transactions[len(ctx.currentBlock.Transactions)-1] = trx
// 	return nil
// }
//
// // Format:
// // DMLOG TRX_EVENT_ATTR <TRX_HASH> <EVENT_INDEX> <KEY> <VALUE>
// func (ctx *parseCtx) eventAttr(params []string) error {
// 	if err := validateChunk(params, 4); err != nil {
// 		return fmt.Errorf("invalid log line length: %w", err)
// 	}
// 	if ctx == nil {
// 		return fmt.Errorf("did not process a BLOCK_BEGIN")
// 	}
// 	if len(ctx.currentBlock.Transactions) == 0 {
// 		return fmt.Errorf("did not process a BEGIN_TRX")
// 	}
//
// 	trx := ctx.currentBlock.Transactions[len(ctx.currentBlock.Transactions)-1]
// 	if trx.Hash != params[0] {
// 		return fmt.Errorf("last transaction hash %q does not match the event trx hash %q", trx.Hash, params[0])
// 	}
//
// 	eventIndex, err := strconv.ParseUint(params[1], 10, 64)
// 	if err != nil {
// 		return fmt.Errorf("invalid event index: %w", err)
// 	}
//
// 	if len(trx.Events) < int(eventIndex) {
// 		return fmt.Errorf("length of events array does not match event index: %d", eventIndex)
// 	}
// 	event := trx.Events[eventIndex]
// 	event.Attributes = append(event.Attributes, &pbcodec.Attribute{
// 		Key:   params[2],
// 		Value: params[3],
// 	})
// 	trx.Events[eventIndex] = event
// 	ctx.currentBlock.Transactions[len(ctx.currentBlock.Transactions)-1] = trx
// 	return nil
// }

// // Format:
// // DMLOG BLOCK_END <HEIGHT> <HASH> <PREV_HASH> <TIMESTAMP> <TRX-COUNT>
// func (ctx *parseCtx) readBlockEnd(params []string) (*pbcodec.Block, error) {
// 	if err := validateChunk(params, 5); err != nil {
// 		return nil, fmt.Errorf("invalid log line length: %w", err)
// 	}
//
// 	if ctx.currentBlock == nil {
// 		return nil, fmt.Errorf("current block not set")
// 	}
//
// 	blockHeight, err := strconv.ParseUint(params[0], 10, 64)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to parse blockNum: %s", err)
// 	}
// 	if blockHeight != ctx.currentBlock.Height {
// 		return nil, fmt.Errorf("end block height does not match active block height, got block height %d but current is block height %d", blockHeight, ctx.currentBlock.Height)
// 	}
//
// 	trxCount, err := strconv.ParseUint(params[4], 10, 64)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to parse blockNum: %s", err)
// 	}
//
// 	if len(ctx.currentBlock.Txs) != int(trxCount) {
// 		return nil, fmt.Errorf("failed expected %d transaction count (had %d) : %s", trxCount, len(ctx.currentBlock.Txs), err)
// 	}
//
// 	timestamp, err := strconv.ParseUint(params[3], 10, 64)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to parse blockNum: %s", err)
// 	}
//
// 	ctx.currentBlock.Hash = params[1]
// 	ctx.currentBlock.PrevHash = params[2]
// 	ctx.currentBlock.Timestamp = timestamp
//
// 	zlog.Debug("console reader read block",
// 		zap.Uint64("height", ctx.currentBlock.Height),
// 		zap.String("hash", ctx.currentBlock.Hash),
// 		zap.String("prev_hash", ctx.currentBlock.PrevHash),
// 		zap.Int("trx_count", len(ctx.currentBlock.Transactions)),
// 	)
// 	return ctx.currentBlock, nil
// }

func validateChunk(params []string, count int) error {
	if len(params) != count {
		return fmt.Errorf("%d fields required but found %d", count, len(params))
	}
	return nil
}
