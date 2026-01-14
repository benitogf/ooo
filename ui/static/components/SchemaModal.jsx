function SchemaModal({ visible, schema, filterPath, description, onClose }) {
  const { useState } = React;
  const { IconX, IconCopy, IconCheck } = window.Icons;
  
  const [copied, setCopied] = useState(false);
  
  if (!visible) return null;

  const handleOverlayClick = (e) => {
    if (e.target === e.currentTarget) onClose();
  };

  const schemaJson = JSON.stringify(schema, null, 2);

  const copySchema = async () => {
    try {
      await navigator.clipboard.writeText(schemaJson);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal schema-modal" onClick={(e) => e.stopPropagation()}>
        <div className="schema-modal-header">
          <div className="schema-modal-titles">
            <div className="modal-title">Schema</div>
            <div className="schema-modal-path">{filterPath}</div>
          </div>
          <button 
            className="modal-close-btn" 
            onClick={onClose}
            title="Close"
          >
            <IconX />
          </button>
        </div>
        
        <div className="schema-modal-content">
          {description && (
            <div className="schema-description">{description}</div>
          )}
          <pre className="schema-json">{schemaJson}</pre>
        </div>
        
        <div className="modal-actions">
          <div></div>
          <button className="btn-confirm" onClick={copySchema}>
            {copied ? <IconCheck /> : <IconCopy />}
            {copied ? 'Copied!' : 'Copy Template'}
          </button>
        </div>
      </div>
    </div>
  );
}

window.SchemaModal = SchemaModal;
