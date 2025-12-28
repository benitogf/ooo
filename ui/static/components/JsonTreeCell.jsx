function JsonTreeCell({ data, maxDepth = 2, maxWidth = 300 }) {
  const { useState } = React;
  const { IconChevronRight, IconChevronDown } = window.Icons;

  // Count total keys/items in nested structure
  const countNested = (value) => {
    if (value === null || value === undefined) return 0;
    if (typeof value !== 'object') return 1;
    if (Array.isArray(value)) {
      return value.reduce((sum, item) => sum + countNested(item), 0);
    }
    return Object.values(value).reduce((sum, v) => sum + countNested(v), 0);
  };

  const renderValue = (value, depth = 0) => {
    if (value === null) return <span className="json-null">null</span>;
    if (value === undefined) return <span className="json-null">-</span>;
    if (typeof value === 'boolean') return <span className={value ? 'text-bool-true' : 'text-bool-false'}>{String(value)}</span>;
    if (typeof value === 'number') {
      const isInt = Number.isInteger(value);
      return <span className={isInt ? 'text-int' : 'text-float'}>{value}</span>;
    }
    if (typeof value === 'string') {
      if (value.length > 30) return <span className="text-string" title={value}>"{value.substring(0, 30)}..."</span>;
      return <span className="text-string">"{value}"</span>;
    }
    if (Array.isArray(value)) {
      return <JsonArray arr={value} depth={depth} maxDepth={maxDepth} />;
    }
    if (typeof value === 'object') {
      return <JsonObject obj={value} depth={depth} maxDepth={maxDepth} />;
    }
    return String(value);
  };

  const JsonObject = ({ obj, depth, maxDepth }) => {
    const [expanded, setExpanded] = useState(false);
    const keys = Object.keys(obj);
    
    if (keys.length === 0) return <span className="json-bracket">{'{}'}</span>;
    
    const totalItems = countNested(obj);
    
    // Always show collapsed by default, with option to expand
    if (!expanded) {
      return (
        <span className="json-collapsed" onClick={(e) => { e.stopPropagation(); setExpanded(true); }}>
          <IconChevronRight className="json-toggle" />
          <span className="json-bracket">{'{'}</span>
          <span className="json-preview">{keys.length} keys{totalItems > keys.length ? ` (${totalItems} total)` : ''}</span>
          <span className="json-bracket">{'}'}</span>
        </span>
      );
    }

    // When expanded but at max depth, show keys only
    if (depth >= maxDepth) {
      return (
        <span className="json-expanded-preview">
          <span className="json-toggle-wrap" onClick={(e) => { e.stopPropagation(); setExpanded(false); }}>
            <IconChevronDown className="json-toggle" />
          </span>
          <span className="json-bracket">{'{'}</span>
          <span className="json-preview">{keys.slice(0, 4).join(', ')}{keys.length > 4 ? `, +${keys.length - 4}` : ''}</span>
          <span className="json-bracket">{'}'}</span>
        </span>
      );
    }

    // Limit displayed keys to avoid huge cells
    const displayKeys = keys.slice(0, 5);
    const hasMore = keys.length > 5;

    return (
      <span className="json-object">
        <span className="json-toggle-wrap" onClick={(e) => { e.stopPropagation(); setExpanded(false); }}>
          <IconChevronDown className="json-toggle" />
        </span>
        <span className="json-bracket">{'{'}</span>
        <span className="json-content">
          {displayKeys.map((key, i) => (
            <span key={key} className="json-entry">
              <span className="json-key">{key}</span>
              <span className="json-colon">: </span>
              {renderValue(obj[key], depth + 1)}
              {(i < displayKeys.length - 1 || hasMore) && <span className="json-comma">,</span>}
            </span>
          ))}
          {hasMore && <span className="json-more">+{keys.length - 5} more</span>}
        </span>
        <span className="json-bracket">{'}'}</span>
      </span>
    );
  };

  const JsonArray = ({ arr, depth, maxDepth }) => {
    const [expanded, setExpanded] = useState(false);
    
    if (arr.length === 0) return <span className="json-bracket">{'[]'}</span>;
    
    const totalItems = countNested(arr);
    
    if (!expanded) {
      return (
        <span className="json-collapsed" onClick={(e) => { e.stopPropagation(); setExpanded(true); }}>
          <IconChevronRight className="json-toggle" />
          <span className="json-bracket">{'['}</span>
          <span className="json-preview">{arr.length} items{totalItems > arr.length ? ` (${totalItems} total)` : ''}</span>
          <span className="json-bracket">{']'}</span>
        </span>
      );
    }

    if (depth >= maxDepth) {
      return (
        <span className="json-expanded-preview">
          <span className="json-toggle-wrap" onClick={(e) => { e.stopPropagation(); setExpanded(false); }}>
            <IconChevronDown className="json-toggle" />
          </span>
          <span className="json-bracket">{'['}</span>
          <span className="json-preview">{arr.length} items</span>
          <span className="json-bracket">{']'}</span>
        </span>
      );
    }

    // Limit displayed items
    const displayItems = arr.slice(0, 3);
    const hasMore = arr.length > 3;

    return (
      <span className="json-array">
        <span className="json-toggle-wrap" onClick={(e) => { e.stopPropagation(); setExpanded(false); }}>
          <IconChevronDown className="json-toggle" />
        </span>
        <span className="json-bracket">{'['}</span>
        <span className="json-content">
          {displayItems.map((item, i) => (
            <span key={i} className="json-entry">
              {renderValue(item, depth + 1)}
              {(i < displayItems.length - 1 || hasMore) && <span className="json-comma">,</span>}
            </span>
          ))}
          {hasMore && <span className="json-more">+{arr.length - 3} more</span>}
        </span>
        <span className="json-bracket">{']'}</span>
      </span>
    );
  };

  return <span className="json-tree-cell">{renderValue(data)}</span>;
}

window.JsonTreeCell = JsonTreeCell;
