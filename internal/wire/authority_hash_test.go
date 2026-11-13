package wire

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
)

// These vectors are shared with the C# tests. Relay decoding/re-encoding must
// preserve the authority hash even when protobuf chooses a different encoding.
func TestAuthorityHashVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/hash-v22.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Name, Frame, Hash string }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("empty authority hash vectors")
	}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(v.Frame)
			if err != nil {
				t.Fatal(err)
			}
			var frame AuthorityFrameMessage
			for i := 0; i < 2; i++ {
				if err := proto.Unmarshal(data, &frame); err != nil {
					t.Fatal(err)
				}
				hash, err := AuthorityFrameHash(&frame)
				if err != nil {
					t.Fatal(err)
				}
				if hex.EncodeToString(hash) != v.Hash {
					t.Fatalf("hash = %x, want %s", hash, v.Hash)
				}
				data, err = proto.Marshal(&frame)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
