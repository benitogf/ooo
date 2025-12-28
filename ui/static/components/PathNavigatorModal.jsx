function PathNavigatorModal({ visible, filterPath, items, onClose }) {
  const { useMemo } = React;
  const { IconX, IconChevronRight } = window.Icons;

  // Extract unique sub-paths from items based on filter pattern
  const subPaths = useMemo(() => {
    if (!items || items.length === 0) return [];
    
    const filterParts = filterPath.split('/');
    const globIndices = [];
    filterParts.forEach((part, idx) => {
      if (part === '*') globIndices.push(idx);
    });
    
    // Only show navigator for multi-glob paths
    if (globIndices.length < 2) return [];
    
    // Get the index of the last glob (which we want to keep as *)
    const lastGlobIndex = globIndices[globIndices.length - 1];
    
    // Extract unique paths up to (but not including) the last glob
    const pathSet = new Set();
    items.forEach(item => {
      if (!item.path) return;
      const pathParts = item.path.split('/');
      // Build path with all globs except the last one filled in
      const subPathParts = filterParts.map((part, idx) => {
        if (idx === lastGlobIndex) return '*';
        if (part === '*' && pathParts[idx]) return pathParts[idx];
        return part;
      });
      pathSet.add(subPathParts.join('/'));
    });
    
    return Array.from(pathSet).sort();
  }, [filterPath, items]);

  if (!visible || subPaths.length === 0) return null;

  const handleOverlayClick = (e) => {
    if (e.target === e.currentTarget) onClose();
  };

  const navigateToPath = (path) => {
    // Pass the original filterPath as 'from' parameter so back button returns here
    window.location.hash = '/storage/keys/live/' + encodeURIComponent(path) + '?from=' + encodeURIComponent(filterPath);
    onClose();
  };

  // Extract display name from path (the non-glob parts that differ)
  const getDisplayName = (path) => {
    const filterParts = filterPath.split('/');
    const pathParts = path.split('/');
    const displayParts = [];
    
    filterParts.forEach((part, idx) => {
      if (part === '*' && pathParts[idx] !== '*') {
        displayParts.push(pathParts[idx]);
      }
    });
    
    return displayParts.join('/');
  };

  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal path-navigator-modal" onClick={(e) => e.stopPropagation()}>
        <div className="path-navigator-header">
          <div className="modal-title">Navigate to Sub-Path</div>
          <button className="modal-close-btn" onClick={onClose} title="Close">
            <IconX />
          </button>
        </div>
        <div className="path-navigator-subtitle">
          Select a path to view its items:
        </div>
        <div className="path-navigator-list">
          {subPaths.map(path => (
            <button 
              key={path} 
              className="path-navigator-item"
              onClick={() => navigateToPath(path)}
            >
              <span className="path-navigator-name">{getDisplayName(path)}</span>
              <span className="path-navigator-path">{path}</span>
              <IconChevronRight />
            </button>
          ))}
        </div>
        <div className="modal-actions">
          <button className="btn-cancel" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
}

window.PathNavigatorModal = PathNavigatorModal;
