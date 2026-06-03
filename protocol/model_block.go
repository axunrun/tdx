package protocol

import (
	"encoding/binary"
	"errors"
	"strings"
)

// 板块文件协议（地域/板块/概念/指数）。参考 pytdx get_block_info + block_reader。
const (
	TypeBlockMeta uint16 = 0x02C5 // 板块文件元信息（大小）
	TypeBlockInfo uint16 = 0x06B9 // 板块文件内容（分块）

	// 通达信板块文件名。
	BlockFileZS = "block_zs.dat" // 指数板块
	BlockFileGN = "block_gn.dat" // 概念板块
	BlockFileFG = "block_fg.dat" // 风格板块（含地域）
	BlockFileHY = "block_hy.dat" // 行业板块（部分服务器提供）
	BlockFile   = "block.dat"    // 一般板块（指数等）

	// 通达信行业归属配置文件（文本，每股票对应通达信行业/申万行业）。
	FileTdxHy = "tdxhy.cfg"

	blockChunk = 0x7530 // 单次下载块大小 30000
)

// BlockMetaResp 板块文件元信息。
type BlockMetaResp struct{ Size uint32 }

// BlockInfoResp 板块文件分块内容。
type BlockInfoResp struct{ Data []byte }

// Block 一个板块及其成分（Codes 为 7 字符，首字符为市场标志：1=沪 0=深）。
type Block struct {
	Name  string
	Type  uint16
	Codes []string
}

type block struct{}

// MBlock 板块协议单例。
var MBlock block

// FrameMeta 构造板块元信息请求帧。
func (block) FrameMeta(file string) *Frame {
	data := make([]byte, 0x2a-2) // 40 字节，文件名 null 填充
	copy(data, file)
	return &Frame{Control: Control01, Type: TypeBlockMeta, Data: data}
}

// FrameInfo 构造板块内容分块请求帧。
func (block) FrameInfo(start, size uint32, file string) *Frame {
	data := make([]byte, 8+0x6e-10) // start(4)+size(4)+filename(100)
	binary.LittleEndian.PutUint32(data[0:4], start)
	binary.LittleEndian.PutUint32(data[4:8], size)
	copy(data[8:], file)
	return &Frame{Control: Control01, Type: TypeBlockInfo, Data: data}
}

// DecodeMeta 解析元信息：前 4 字节为文件大小。
func (block) DecodeMeta(bs []byte) (*BlockMetaResp, error) {
	if len(bs) < 4 {
		return nil, errors.New("block meta 数据不足")
	}
	return &BlockMetaResp{Size: Uint32(bs[:4])}, nil
}

// DecodeInfo 解析分块内容：去掉前 4 字节（块长度），其余为文件内容。
func (block) DecodeInfo(bs []byte) (*BlockInfoResp, error) {
	if len(bs) < 4 {
		return &BlockInfoResp{Data: nil}, nil
	}
	return &BlockInfoResp{Data: bs[4:]}, nil
}

// TdxHy 一只股票的行业归属（来自 tdxhy.cfg）。
type TdxHy struct {
	Market uint8  // 0=深 1=沪
	Code   string // 6 位代码
	TdxHy  string // 通达信新行业代码（T 前缀）
	SwHy   string // 申万行业代码（X 前缀）
}

// ParseTdxHy 解析 tdxhy.cfg 文本：每行 `市场|代码|通达信行业|||申万行业`。
func ParseTdxHy(data []byte) []*TdxHy {
	lines := strings.Split(string(data), "\n")
	out := make([]*TdxHy, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" {
			continue
		}
		f := strings.Split(ln, "|")
		if len(f) < 3 || len(f[0]) == 0 {
			continue
		}
		hy := &TdxHy{Market: uint8(f[0][0] - '0'), Code: f[1], TdxHy: f[2]}
		if len(f) >= 6 {
			hy.SwHy = f[5]
		} else {
			hy.SwHy = f[len(f)-1]
		}
		out = append(out, hy)
	}
	return out
}

// ParseBlockFile 解析完整板块文件 → 板块列表。
// 格式：偏移 384 处 uint16 板块数；每块 = 名称(9,GBK) + 成分数(2) + 类型(2) + 成分(400×7)，定长 2813。
func ParseBlockFile(data []byte) []*Block {
	if len(data) < 386 {
		return nil
	}
	pos := 384
	num := int(Uint16(data[pos : pos+2]))
	pos += 2
	out := make([]*Block, 0, num)
	for i := 0; i < num; i++ {
		if pos+13 > len(data) {
			break
		}
		name := strings.TrimRight(string(UTF8ToGBK(data[pos:pos+9])), "\x00")
		pos += 9
		stockCount := int(Uint16(data[pos : pos+2]))
		blockType := Uint16(data[pos+2 : pos+4])
		pos += 4
		begin := pos
		codes := make([]string, 0, stockCount)
		for j := 0; j < stockCount; j++ {
			if pos+7 > len(data) {
				break
			}
			c := strings.TrimRight(string(data[pos:pos+7]), "\x00")
			if c != "" {
				codes = append(codes, c)
			}
			pos += 7
		}
		pos = begin + 400*7 // 每块成分区固定 2800 字节
		out = append(out, &Block{Name: name, Type: blockType, Codes: codes})
	}
	return out
}
