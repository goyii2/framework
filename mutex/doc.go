// Package mutex provides name-keyed mutual exclusion with a process-local
// and a redis-backed driver; it corresponds to Yii2's yii\mutex subsystem.
//
// Unlike Yii2 the locks are not reentrant: acquiring a name the caller
// already holds blocks (Lock) or fails (TryLock) instead of stacking a
// second acquisition. Reentrancy over plain name locks is unsafe in Go —
// a release from one goroutine would free a lock another goroutine still
// believes it holds — and the token-checked release of the redis driver
// could not tell two acquisitions of the same name apart.
package mutex
