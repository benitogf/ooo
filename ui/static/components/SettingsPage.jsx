function SettingsPage() {
  const { useState, useEffect, useCallback } = React;
  const { IconRefresh } = window.Icons;
  
  const [items, setItems] = useState([]);

  const loadData = useCallback(async () => {
    try {
      const res = await fetch('/?api=info');
      const info = await res.json();
      setItems(Object.entries(info).map(([key, value]) => ({
        key,
        value: formatValue(value)
      })));
    } catch (err) {
      console.error('Failed to load server info:', err);
    }
  }, []);

  const formatValue = (val) => {
    if (typeof val === 'boolean') return val ? 'Yes' : 'No';
    if (typeof val === 'number' && val > 1000000) return (val / 1000000000) + 's';
    return String(val);
  };

  useEffect(() => {
    loadData();
  }, [loadData]);

  return (
    <div className="container">
      <div className="page-header">
        <h1 className="page-title">Settings</h1>
        <div className="header-actions">
          <button className="refresh-btn" onClick={loadData} title="Refresh">
            <IconRefresh />
          </button>
        </div>
      </div>
      <div className="info-grid">
        {items.map(item => (
          <div key={item.key} className="info-item">
            <div className="info-label">{item.key}</div>
            <div className="info-value">{item.value}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

window.SettingsPage = SettingsPage;
