package osdp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go"
)

// TestFacadeIsUsableFromOutside exercises the public surface the way a consumer
// would. It lives in package osdp_test, not package osdp, so it can only touch
// what is genuinely exported -- which is the claim the re-export by type alias
// is making.
func TestFacadeIsUsableFromOutside(t *testing.T) {
	// osdp_POLL, control panel to PD 0x00, CRC, sequence 1.
	wire := []byte{0x53, 0x00, 0x08, 0x00, 0x05, 0x60, 0xDA, 0x99}

	var f osdp.Frame
	f, err := osdp.Decode(context.Background(), wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	var addr osdp.Address = f.Address
	if addr != 0x00 {
		t.Errorf("Address = 0x%02X, want 0x00", byte(addr))
	}
	if f.Control.Scheme() != osdp.SchemeCRC16 {
		t.Error("the aliased Scheme constant does not compare equal")
	}
	if f.Code != 0x60 {
		t.Errorf("Code = 0x%02X, want 0x60", f.Code)
	}
}

// TestFacadeTypesAreAliasesNotWrappers is the load-bearing assertion.
//
// A wrapper type would make a value from the facade incompatible with the
// interfaces declared inside the module, closing every extension point. An
// alias is the same type, so composing a frame here and encoding it through the
// facade must work without conversion.
func TestFacadeTypesAreAliasesNotWrappers(t *testing.T) {
	ctx := context.Background()

	f := osdp.Frame{
		Address: 0x02,
		Control: osdp.NewControl(1, osdp.SchemeCRC16, false),
		Code:    0x60,
	}
	f.Seal()

	wire, err := f.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if wire[0] != osdp.SOM {
		t.Errorf("first octet = 0x%02X, want SOM 0x%02X", wire[0], osdp.SOM)
	}

	got, err := osdp.Decode(ctx, wire)
	if err != nil {
		t.Fatalf("Decode of a frame composed through the facade: %v", err)
	}
	if got.Address != f.Address || got.Code != f.Code {
		t.Error("a frame composed through the facade did not survive a round trip")
	}
}

func TestFacadeErrorsMatchWithErrorsIs(t *testing.T) {
	corrupt := []byte{0x53, 0x00, 0x08, 0x00, 0x05, 0x60, 0x00, 0x00}

	f, err := osdp.Decode(context.Background(), corrupt)
	if !errors.Is(err, osdp.ErrBadCheck) {
		t.Fatalf("Decode = %v, want ErrBadCheck", err)
	}
	if f.Code != 0x60 {
		t.Error("a frame with a bad check must still be returned populated")
	}

	if _, err := osdp.Decode(context.Background(), nil); !errors.Is(err, osdp.ErrShortBuffer) {
		t.Errorf("Decode(nil) = %v, want ErrShortBuffer", err)
	}
}
