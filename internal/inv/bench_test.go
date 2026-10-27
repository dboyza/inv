package inv

import "testing"

func BenchmarkBuildFromCache(b *testing.B) {
	snap, ok := LoadCache()
	if !ok {
		b.Skip("no cache")
	}
	env := NewEnv()
	for b.Loop() {
		Build(env, snap)
	}
}
