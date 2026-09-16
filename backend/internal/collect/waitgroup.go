package collect

import "sync"

type syncWaitGroup struct{ wg sync.WaitGroup }

func (s *syncWaitGroup) Go(f func()) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		f()
	}()
}

func (s *syncWaitGroup) Wait() { s.wg.Wait() }
