package workspaces

import "context"

type fakeCryptor struct {
	encryptCalls int
}

func (f *fakeCryptor) DecryptProjectFile(_ context.Context, _ int64, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func (f *fakeCryptor) EncryptProjectFile(_ context.Context, _ int64, _ string, data []byte) ([]byte, bool, error) {
	f.encryptCalls++
	out := append([]byte("enc:"), data...)
	return out, true, nil
}
