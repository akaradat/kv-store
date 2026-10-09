package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/kv"
)

var _ = Describe("Store", func() {
	var s *Store

	BeforeEach(func() {
		s = New()
	})

	conflict := func(err error) *kv.VersionConflictError {
		var vc *kv.VersionConflictError
		Expect(errors.As(err, &vc)).To(BeTrue(), "want a version conflict, got %v", err)
		return vc
	}

	Describe("Get", func() {
		It("reports a missing key as not found", Label("R-ERR-3"), func() {
			_, err := s.Get("nope")
			Expect(err).To(MatchError(kv.ErrNotFound))
		})

		It("reports an entry with no committed value as not found", Label("R-IFV-4"), func() {
			s.entries.Store("pending", &entry{})
			_, err := s.Get("pending")
			Expect(err).To(MatchError(kv.ErrNotFound))
		})

		It("treats keys as case-sensitive", Label("R-KEY-4"), func() {
			_, err := s.Put("user:1", kv.Unconditional, []byte(`{}`))
			Expect(err).NotTo(HaveOccurred())
			_, err = s.Get("User:1")
			Expect(err).To(MatchError(kv.ErrNotFound))
		})
	})

	Describe("Put", func() {
		It("creates a missing key at version 1", Label("R-VER-1"), func() {
			e, err := s.Put("a", kv.Unconditional, []byte(`{"n":1}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(e).To(Equal(kv.Entry{Key: "a", Value: []byte(`{"n":1}`), Version: 1}))
		})

		It("replaces the whole value and raises the version by 1, even when the value is the same", Label("R-VER-2"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"n":1,"m":2}`))
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"n":1}`))
			e, err := s.Put("a", kv.Unconditional, []byte(`{"n":1}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(e.Version).To(BeEquivalentTo(3))

			got, err := s.Get("a")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got.Value)).To(Equal(`{"n":1}`))
		})

		It("keeps the exact bytes it was given", Label("R-FID-1"), func() {
			body := `{"n":12345678901234567890, "f":1.0,"name":1,"name":2,"s":"<A>"}`
			_, _ = s.Put("a", kv.Unconditional, []byte(body))
			got, _ := s.Get("a")
			Expect(string(got.Value)).To(Equal(body))
		})

		It("writes when ifVersion matches", Label("R-IFV-3"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"n":1}`))
			e, err := s.Put("a", 1, []byte(`{"n":2}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(e.Version).To(BeEquivalentTo(2))
		})

		It("rejects a stale ifVersion and changes nothing", Label("R-IFV-3", "R-VER-3"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"n":1}`))
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"n":2}`))

			_, err := s.Put("a", 1, []byte(`{"n":99}`))
			Expect(conflict(err).Current).To(BeEquivalentTo(2))

			got, _ := s.Get("a")
			Expect(got).To(Equal(kv.Entry{Key: "a", Value: []byte(`{"n":2}`), Version: 2}))
		})

		It("rejects ifVersion on a missing key without creating it", Label("R-IFV-4"), func() {
			_, err := s.Put("a", 1, []byte(`{}`))
			Expect(conflict(err).Current).To(BeZero())

			_, ok := s.entries.Load("a")
			Expect(ok).To(BeFalse())
		})

		It("rejects ifVersion on an entry whose first write has not landed", Label("R-IFV-4"), func() {
			s.entries.Store("pending", &entry{})
			_, err := s.Put("pending", 1, []byte(`{}`))
			Expect(conflict(err).Current).To(BeZero())
		})
	})

	Describe("Patch", func() {
		It("creates a missing key with the delta's exact bytes", Label("R-VER-1", "R-FID-1"), func() {
			e, err := s.Patch("a", kv.Unconditional, []byte(`{ "b" : 1.0 }`))
			Expect(err).NotTo(HaveOccurred())
			Expect(e).To(Equal(kv.Entry{Key: "a", Value: []byte(`{ "b" : 1.0 }`), Version: 1}))
		})

		It("merges into an existing value and raises the version by 1", Label("R-PATCH-1", "R-VER-2"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"name":"Ari","points":10}`))
			e, err := s.Patch("a", kv.Unconditional, []byte(`{"points":30,"rank":"gold"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(e.Version).To(BeEquivalentTo(2))
			Expect(string(e.Value)).To(MatchJSON(`{"name":"Ari","points":30,"rank":"gold"}`))
		})

		It("raises the version for an empty delta", Label("R-PATCH-4", "R-VER-2"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"x":1}`))
			e, err := s.Patch("a", kv.Unconditional, []byte(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(e.Version).To(BeEquivalentTo(2))
			Expect(string(e.Value)).To(MatchJSON(`{"x":1}`))
		})

		It("rejects a stale ifVersion and changes nothing", Label("R-IFV-3", "R-VER-3"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"x":1}`))
			_, err := s.Patch("a", 5, []byte(`{"y":1}`))
			Expect(conflict(err).Current).To(BeEquivalentTo(1))

			got, _ := s.Get("a")
			Expect(got.Version).To(BeEquivalentTo(1))
			Expect(string(got.Value)).To(Equal(`{"x":1}`))
		})

		It("rejects ifVersion on a missing key without creating it", Label("R-IFV-4"), func() {
			_, err := s.Patch("a", 1, []byte(`{}`))
			Expect(conflict(err).Current).To(BeZero())
			_, ok := s.entries.Load("a")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("Range", func() {
		It("visits nothing on an empty store", Label("R-LIST-5"), func() {
			var keys []string
			s.Range(func(k string) bool { keys = append(keys, k); return true })
			Expect(keys).To(BeEmpty())
		})

		It("visits each written key once and skips entries with no committed value", Label("R-LIST-3"), func() {
			for range 5 {
				_, _ = s.Put("a", kv.Unconditional, []byte(`{}`))
			}
			_, _ = s.Patch("b", kv.Unconditional, []byte(`{}`))
			s.entries.Store("pending", &entry{})

			var keys []string
			s.Range(func(k string) bool { keys = append(keys, k); return true })
			Expect(keys).To(ConsistOf("a", "b"))
		})

		It("stops when fn returns false", Label("R-LIST-6"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{}`))
			_, _ = s.Put("b", kv.Unconditional, []byte(`{}`))
			calls := 0
			s.Range(func(string) bool { calls++; return false })
			Expect(calls).To(Equal(1))
		})

		It("lets writes complete while it runs and lists every key that existed before", Label("R-LIST-6", "R-CON-7"), func() {
			before := make([]string, 100)
			for i := range before {
				before[i] = fmt.Sprintf("old-%d", i)
				_, _ = s.Put(before[i], kv.Unconditional, []byte(`{}`))
			}

			var listed []string
			writes := 0
			s.Range(func(k string) bool {
				listed = append(listed, k)
				_, err := s.Put(k, kv.Unconditional, []byte(`{"touched":true}`))
				Expect(err).NotTo(HaveOccurred())
				writes++
				if strings.HasPrefix(k, "old-") {
					_, err = s.Put("new-"+k, kv.Unconditional, []byte(`{}`))
					Expect(err).NotTo(HaveOccurred())
					writes++
				}
				return true
			})

			Expect(writes).To(BeNumerically(">=", 200))
			for _, k := range before {
				Expect(slices.Contains(listed, k)).To(BeTrue(), "missing %s", k)
			}
			slices.Sort(listed)
			Expect(slices.Compact(slices.Clone(listed))).To(HaveLen(len(listed)), "a key was listed twice")
		})
	})

	Describe("concurrency", func() {
		It("does not make a write to one key wait for a held lock on another", Label("R-CON-2"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{}`))
			a, _ := s.load("a")
			a.mu.Lock()
			unlocked := false
			DeferCleanup(func() {
				if !unlocked {
					a.mu.Unlock()
				}
			})

			otherKey := make(chan struct{})
			go func() {
				_, _ = s.Put("b", kv.Unconditional, []byte(`{}`))
				close(otherKey)
			}()
			Eventually(otherKey).Should(BeClosed())

			sameKey := make(chan struct{})
			go func() {
				_, _ = s.Put("a", kv.Unconditional, []byte(`{}`))
				close(sameKey)
			}()
			Consistently(sameKey, 50*time.Millisecond).ShouldNot(BeClosed())
			a.mu.Unlock()
			unlocked = true
			Eventually(sameKey).Should(BeClosed())
		})

		It("loses no increments when 3 clients do 100 guarded read-modify-writes each", Label("R-IFV-6", "R-CON-1", "R-CON-4"), func() {
			_, _ = s.Put("counter", kv.Unconditional, []byte(`{"count":0}`))

			const clients, increments = 3, 100
			var retries atomic.Int64
			var unexpected atomic.Value
			var wg sync.WaitGroup
			start := make(chan struct{})
			for range clients {
				wg.Go(func() {
					<-start
					for done := 0; done < increments; {
						cur, err := s.Get("counter")
						if err != nil {
							unexpected.Store(err)
							return
						}
						var v struct{ Count int }
						if err := json.Unmarshal(cur.Value, &v); err != nil {
							unexpected.Store(err)
							return
						}
						next := fmt.Appendf(nil, `{"count":%d}`, v.Count+1)
						_, err = s.Put("counter", cur.Version, next)
						var vc *kv.VersionConflictError
						switch {
						case err == nil:
							done++
						case errors.As(err, &vc):
							retries.Add(1)
						default:
							unexpected.Store(err)
							return
						}
					}
				})
			}
			close(start)
			wg.Wait()

			Expect(unexpected.Load()).To(BeNil())
			got, err := s.Get("counter")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got.Value)).To(MatchJSON(`{"count":300}`))
			Expect(got.Version).To(BeEquivalentTo(1 + clients*increments))
			GinkgoWriter.Printf("retries: %d\n", retries.Load())
		})

		It("lets exactly one of many writes with the same ifVersion succeed", Label("R-CON-4", "R-IFV-6"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{}`))

			const writers = 50
			var ok, conflicts atomic.Int64
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range writers {
				wg.Go(func() {
					<-start
					_, err := s.Put("a", 1, fmt.Appendf(nil, `{"w":%d}`, i))
					if err == nil {
						ok.Add(1)
					} else {
						conflicts.Add(1)
					}
				})
			}
			close(start)
			wg.Wait()

			Expect(ok.Load()).To(BeEquivalentTo(1))
			Expect(conflicts.Load()).To(BeEquivalentTo(writers - 1))
		})

		It("keeps every field from concurrent patches of different fields", Label("R-CON-4"), func() {
			const writers = 50
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range writers {
				wg.Go(func() {
					<-start
					_, _ = s.Patch("a", kv.Unconditional, fmt.Appendf(nil, `{"f%d":%d}`, i, i))
				})
			}
			close(start)
			wg.Wait()

			got, err := s.Get("a")
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Version).To(BeEquivalentTo(writers))
			var fields map[string]int
			Expect(json.Unmarshal(got.Value, &fields)).To(Succeed())
			Expect(fields).To(HaveLen(writers))
		})

		It("returns to each concurrent writer the value and version it committed", Label("R-CON-6", "R-CON-1"), func() {
			const writers = 50
			results := make([]kv.Entry, writers)
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range writers {
				wg.Go(func() {
					<-start
					results[i], _ = s.Put("a", kv.Unconditional, fmt.Appendf(nil, `{"w":%d}`, i))
				})
			}
			close(start)
			wg.Wait()

			versions := make([]uint64, writers)
			for i, r := range results {
				Expect(string(r.Value)).To(Equal(fmt.Sprintf(`{"w":%d}`, i)))
				versions[i] = r.Version
			}
			slices.Sort(versions)
			for i, v := range versions {
				Expect(v).To(BeEquivalentTo(i+1), "versions must be 1..%d with no gaps or repeats", writers)
			}
		})

		It("never returns a value with another write's version", Label("R-CON-3"), func() {
			_, _ = s.Put("a", kv.Unconditional, []byte(`{"v":1}`))

			const writers, writes = 4, 200
			var torn atomic.Int64
			var wg sync.WaitGroup
			stop := make(chan struct{})
			for range writers {
				wg.Go(func() {
					for done := 0; done < writes; {
						cur, _ := s.Get("a")
						if _, err := s.Put("a", cur.Version, fmt.Appendf(nil, `{"v":%d}`, cur.Version+1)); err == nil {
							done++
						}
					}
				})
			}
			var readers sync.WaitGroup
			for range 4 {
				readers.Go(func() {
					for {
						select {
						case <-stop:
							return
						default:
						}
						cur, _ := s.Get("a")
						if string(cur.Value) != fmt.Sprintf(`{"v":%d}`, cur.Version) {
							torn.Add(1)
						}
					}
				})
			}
			wg.Wait()
			close(stop)
			readers.Wait()

			Expect(torn.Load()).To(BeZero())
		})
	})
})
