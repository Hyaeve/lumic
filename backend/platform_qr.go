package main

// platformQRGuard is checked under the store lock before credentials are saved.
type platformQRGuard func(BilibiliConfig) bool

type platformQRError struct {
	status  int
	message string
}

func (b *BilibiliStore) discardPlatformQR(source Source, id string) {
	b.Lock()
	defer b.Unlock()
	switch source {
	case SourceBilibili:
		delete(b.bilibiliQR, id)
		delete(b.bilibiliClients, id)
	case SourceWeibo:
		delete(b.weiboQR, id)
		delete(b.weiboClients, id)
	}
}
