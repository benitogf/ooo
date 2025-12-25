package storage

// WatchStorageNoop drains all events from a storage's sharded watcher channels.
// Use this for extra storages that are not directly hooked to a server.
func WatchStorageNoop(dataStore Database) {
	shardedWatcher := dataStore.WatchSharded()
	if shardedWatcher == nil {
		return
	}
	for i := 0; i < shardedWatcher.Count(); i++ {
		go func(ch StorageChan) {
			for {
				_, ok := <-ch
				if !ok || !dataStore.Active() {
					return
				}
			}
		}(shardedWatcher.Shard(i))
	}
}
