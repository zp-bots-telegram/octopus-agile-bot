package cryptobox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var key32 = []byte("0123456789abcdef0123456789abcdef") // 32 bytes

func TestRoundTrip(t *testing.T) {
	c, err := New(key32)
	require.NoError(t, err)

	for _, plain := range [][]byte{
		[]byte("sk_live_something"),
		[]byte("a longer plaintext with some spaces and \x00 null bytes"),
	} {
		enc, err := c.Encrypt(plain)
		require.NoError(t, err)
		got, err := c.Decrypt(enc)
		require.NoError(t, err)
		assert.Equal(t, plain, got)
	}
}

func TestEncryptIsNondeterministic(t *testing.T) {
	c, _ := New(key32)
	a, _ := c.Encrypt([]byte("hello"))
	b, _ := c.Encrypt([]byte("hello"))
	assert.NotEqual(t, a, b)
}

func TestDecryptDetectsTamper(t *testing.T) {
	c, _ := New(key32)
	enc, _ := c.Encrypt([]byte("hello"))
	enc[len(enc)-1] ^= 0x01
	_, err := c.Decrypt(enc)
	assert.Error(t, err)
}

func TestWrongKey(t *testing.T) {
	a, _ := New(key32)
	b, _ := New([]byte("fedcba9876543210fedcba9876543210"))
	enc, _ := a.Encrypt([]byte("hello"))
	_, err := b.Decrypt(enc)
	assert.Error(t, err)
}

func TestKeyLength(t *testing.T) {
	_, err := New([]byte("short"))
	assert.ErrorIs(t, err, ErrKeySize)
}

func TestDecryptTooShort(t *testing.T) {
	c, _ := New(key32)
	_, err := c.Decrypt([]byte("abc"))
	assert.Error(t, err)
}
