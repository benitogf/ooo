function PushDialog({ visible, filterPath, existingKeys = [], onClose, onSuccess, onEditKey }) {
  const { useState, useRef, useEffect, useMemo } = React;
  const JsonEditorWrapper = window.JsonEditorWrapper;
  const { IconEdit, IconX } = window.Icons;
  
  const [pushing, setPushing] = useState(false);
  const [error, setError] = useState('');
  const [pathSegments, setPathSegments] = useState({});
  const [customIndex, setCustomIndex] = useState('');
  const editorRef = useRef(null);

  // Parse filter path to find glob positions
  const globInfo = useMemo(() => {
    const parts = filterPath.split('/');
    const globs = [];
    parts.forEach((part, idx) => {
      if (part === '*') {
        globs.push({ index: idx, label: parts.slice(0, idx).join('/') || 'root' });
      }
    });
    return { parts, globs, hasMultipleGlobs: globs.length > 1 };
  }, [filterPath]);

  // Check if the current path matches an existing key
  const existingKeyMatch = useMemo(() => {
    if (!customIndex) return null;
    // Build target path inline to avoid function hoisting issues
    const parts = [...globInfo.parts];
    globInfo.globs.forEach((glob, idx) => {
      if (idx < globInfo.globs.length - 1) {
        const value = pathSegments[glob.index];
        if (value) parts[glob.index] = value;
      } else {
        if (customIndex) {
          parts[glob.index] = customIndex;
        }
      }
    });
    const targetPath = parts.join('/');
    if (targetPath.includes('*')) return null;
    return existingKeys.find(key => key === targetPath);
  }, [customIndex, pathSegments, existingKeys, globInfo]);

  useEffect(() => {
    if (visible) {
      setPushing(false);
      setError('');
      setPathSegments({});
      setCustomIndex('');
    }
  }, [visible]);

  if (!visible) return null;

  const handleOverlayClick = (e) => {
    if (e.target === e.currentTarget && !pushing) onClose();
  };

  const buildTargetPath = () => {
    const parts = [...globInfo.parts];
    
    // Fill in path segments for all globs except the last one
    globInfo.globs.forEach((glob, idx) => {
      if (idx < globInfo.globs.length - 1) {
        // Required segment - must be filled
        const value = pathSegments[glob.index];
        if (!value) return null;
        parts[glob.index] = value;
      } else {
        // Last glob - use custom index if provided, otherwise keep *
        if (customIndex) {
          parts[glob.index] = customIndex;
        }
        // If no custom index, keep * for auto-generation
      }
    });
    
    return parts.join('/');
  };

  const validatePath = () => {
    // Check all required segments (all except last glob) are filled
    for (let i = 0; i < globInfo.globs.length - 1; i++) {
      const glob = globInfo.globs[i];
      if (!pathSegments[glob.index] || !pathSegments[glob.index].trim()) {
        return false;
      }
    }
    return true;
  };

  const push = async () => {
    if (globInfo.hasMultipleGlobs && !validatePath()) {
      setError('Please fill in all required path segments');
      return;
    }
    
    setPushing(true);
    setError('');
    
    const minDelay = new Promise(r => setTimeout(r, 500));
    const targetPath = buildTargetPath();
    
    try {
      const content = editorRef.current?.getContent() || { json: {} };
      const dataToSave = content.json !== undefined 
        ? JSON.stringify(content.json) 
        : content.text !== undefined ? content.text : '{}';
      
      const [res] = await Promise.all([
        fetch('/' + targetPath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: dataToSave
        }),
        minDelay
      ]);
      
      if (!res.ok) throw new Error(await res.text());
      
      setPushing(false);
      if (onSuccess) onSuccess();
      onClose();
    } catch (err) {
      setPushing(false);
      setError('Failed: ' + err.message);
    }
  };

  const updateSegment = (index, value) => {
    setPathSegments(prev => ({ ...prev, [index]: value }));
  };

  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal push-dialog" onClick={(e) => e.stopPropagation()}>
        <div className="push-dialog-header">
          <div className="modal-title">Push to: {filterPath}</div>
          <button 
            className="modal-close-btn" 
            onClick={onClose}
            disabled={pushing}
            title="Close"
          >
            <IconX />
          </button>
        </div>
        
        {globInfo.globs.length > 0 && !pushing && (
          <div className="push-dialog-path-inputs">
            {globInfo.globs.map((glob, idx) => {
              const isLast = idx === globInfo.globs.length - 1;
              const prefix = globInfo.parts.slice(0, glob.index).join('/');
              return (
                <div key={glob.index} className="path-input-row">
                  <label>
                    <span className="path-prefix">{prefix}/</span>
                    {isLast ? (
                      <input
                        type="text"
                        placeholder="(auto-generate)"
                        value={customIndex}
                        onChange={(e) => setCustomIndex(e.target.value)}
                        className="path-input optional"
                      />
                    ) : (
                      <input
                        type="text"
                        placeholder="required"
                        value={pathSegments[glob.index] || ''}
                        onChange={(e) => updateSegment(glob.index, e.target.value)}
                        className="path-input required"
                      />
                    )}
                    {isLast && <span className="path-hint">(optional)</span>}
                  </label>
                </div>
              );
            })}
          </div>
        )}
        
        {pushing ? (
          <div className="push-dialog-loading">
            <div className="spinner"></div>
            <div>Pushing data...</div>
          </div>
        ) : (
          <div className="push-dialog-editor">
            <JsonEditorWrapper content={{}} editorRef={editorRef} />
          </div>
        )}
        
        {error && <div className="modal-error">{error}</div>}
        
        {existingKeyMatch && (
          <div className="modal-warning">
            Key already exists. Use Edit to modify it.
          </div>
        )}
        
        <div className="modal-actions">
          <button className="btn-cancel" onClick={onClose} disabled={pushing}>
            Cancel
          </button>
          {existingKeyMatch ? (
            <button 
              className="btn-confirm" 
              onClick={() => {
                onClose();
                if (onEditKey) onEditKey(existingKeyMatch);
              }}
            >
              <IconEdit /> Edit Key
            </button>
          ) : (
            <button 
              className="btn-confirm" 
              onClick={push}
              disabled={pushing}
            >
              {pushing ? 'Pushing...' : 'Push Data'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

window.PushDialog = PushDialog;
