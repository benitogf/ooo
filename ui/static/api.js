// API wrapper for ooo-client WebSocket functionality
// Simplified version without auth for explorer use

const Api = (function() {
  const getWsProtocol = () => window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const getHost = () => window.location.host;

  // Subscribe to a key or glob pattern
  // callbacks: { onMessage, onOpen, onClose, onError }
  function subscribe(path, callbacks) {
    const { onMessage, onOpen, onClose, onError } = callbacks || {};
    // For clock (empty path), pass just the host without trailing slash
    // ooo-client detects clock mode when urlSplit.length === 1
    const fullPath = path ? getHost() + '/' + path : getHost();
    console.log('Api.subscribe fullPath:', fullPath, 'path:', path);
    const socket = ooo(fullPath, window.location.protocol === 'https:');
    
    socket.onmessage = (data) => {
      if (onMessage) onMessage(data);
    };
    
    socket.onerror = (e) => {
      console.warn('WebSocket error:', path, e);
      if (onError) onError(e);
    };
    
    socket.onopen = () => {
      console.log('WebSocket connected:', path || '(root)');
      if (onOpen) onOpen();
    };
    
    socket.onclose = () => {
      console.log('WebSocket closed:', path || '(root)');
      if (onClose) onClose();
    };
    
    return socket;
  }

  // React hook for subscribing to a path
  function useSubscribe(path) {
    const { useState, useEffect, useRef } = React;
    const [data, setData] = useState(null);
    const [version, setVersion] = useState(0);
    const [connected, setConnected] = useState(false);
    const [error, setError] = useState(null);
    const socketRef = useRef(null);

    useEffect(() => {
      if (!path) {
        setData(null);
        setConnected(false);
        return;
      }

      // Close existing socket if path changed
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }

      const socket = ooo(getHost() + '/' + path, window.location.protocol === 'https:');
      socketRef.current = socket;

      socket.onopen = () => {
        setConnected(true);
        setError(null);
      };

      socket.onmessage = (newData) => {
        console.log('useSubscribe received:', path, newData);
        // Create a new object to ensure React detects the change
        setData({ ...newData, _version: Date.now() });
        setVersion(v => v + 1);
      };

      socket.onerror = (e) => {
        console.warn('WebSocket error:', path, e);
        setError(e);
        setConnected(false);
      };

      socket.onclose = () => {
        setConnected(false);
      };

      return () => {
        if (socketRef.current) {
          socketRef.current.close();
          socketRef.current = null;
        }
      };
    }, [path]);

    return { data, connected, error, version, socket: socketRef.current };
  }

  // React hook for subscribing to a glob pattern with item tracking
  function useSubscribeGlob(path) {
    const { useState, useEffect, useRef, useCallback } = React;
    const [items, setItems] = useState([]);
    const [connected, setConnected] = useState(false);
    const [error, setError] = useState(null);
    const [lastUpdate, setLastUpdate] = useState({ type: null, index: null });
    const socketRef = useRef(null);
    const itemsRef = useRef([]);

    useEffect(() => {
      if (!path) {
        setItems([]);
        setConnected(false);
        return;
      }

      // Close existing socket if path changed
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }

      const socket = ooo(getHost() + '/' + path, window.location.protocol === 'https:');
      socketRef.current = socket;

      socket.onopen = () => {
        setConnected(true);
        setError(null);
      };

      socket.onmessage = (newData) => {
        if (!newData) return;
        
        // newData is an array of items for glob subscriptions
        const rawItems = Array.isArray(newData) ? newData : [newData];
        
        // Deduplicate by index (keep first occurrence) and sort by created timestamp desc (newest first)
        const seen = new Set();
        const newItems = rawItems
          .filter(item => {
            if (!item || !item.index) return false;
            if (seen.has(item.index)) return false;
            seen.add(item.index);
            return true;
          })
          .sort((a, b) => (b.created || 0) - (a.created || 0));
        
        const currentItems = itemsRef.current;
        
        // Determine what changed
        let updateType = null;
        let updateIndex = null;
        
        if (currentItems.length === 0) {
          // Initial load - but check if it's a single item (could be first add after initial empty)
          if (newItems.length === 1) {
            updateType = 'added';
            updateIndex = newItems[0].index;
          } else {
            updateType = 'initial';
          }
        } else {
          // Build a map for O(1) lookup
          const currentMap = new Map(currentItems.map(i => [i.index, i]));
          
          for (const newItem of newItems) {
            const existing = currentMap.get(newItem.index);
            if (!existing) {
              // New item added
              updateType = 'added';
              updateIndex = newItem.index;
              break; // Prioritize added over updated
            } else if (existing.updated !== newItem.updated) {
              // Updated item
              updateType = 'updated';
              updateIndex = newItem.index;
              // Don't break - continue looking for added items
            }
          }
          
          // Check for deletions if nothing else found
          if (!updateType) {
            const newMap = new Map(newItems.map(i => [i.index, i]));
            for (const currentItem of currentItems) {
              if (!newMap.has(currentItem.index)) {
                updateType = 'deleted';
                updateIndex = currentItem.index;
                break;
              }
            }
          }
        }
        
        itemsRef.current = newItems;
        setItems(newItems);
        
        if (updateType && updateType !== 'initial' && updateIndex) {
          setLastUpdate({ type: updateType, index: updateIndex, timestamp: Date.now() });
        }
      };

      socket.onerror = (e) => {
        console.warn('WebSocket error:', path, e);
        setError(e);
        setConnected(false);
      };

      socket.onclose = () => {
        setConnected(false);
      };

      return () => {
        if (socketRef.current) {
          socketRef.current.close();
          socketRef.current = null;
        }
      };
    }, [path]);

    const clearLastUpdate = useCallback(() => {
      setLastUpdate({ type: null, index: null });
    }, []);

    return { items, connected, error, lastUpdate, clearLastUpdate, socket: socketRef.current };
  }

  return {
    subscribe,
    useSubscribe,
    useSubscribeGlob,
    getHost,
    getWsProtocol
  };
})();

window.Api = Api;
