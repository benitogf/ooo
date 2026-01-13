function PivotStatus({ onClose }) {
    const { useState, useEffect } = React;
    const { IconX, IconServer, IconCloud, IconCloudOff, IconCheck, IconAlertCircle } = window.Icons;

    // Use ooo-client subscription for real-time updates
    const { data: wsData, connected, error: wsError } = Api.useSubscribe('pivot/status');
    
    const [pivotInfo, setPivotInfo] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    // Initial fetch
    useEffect(() => {
        fetch('/?api=pivot')
            .then(res => res.json())
            .then(data => {
                setPivotInfo(data);
                setLoading(false);
                setError(null);
            })
            .catch(err => {
                setError(err.message);
                setLoading(false);
            });
    }, []);

    // Update from WebSocket data
    useEffect(() => {
        if (wsData && wsData.data) {
            setPivotInfo(wsData.data);
            setLoading(false);
            setError(null);
        }
    }, [wsData]);

    // Show error when connection is lost (only after we've had data)
    useEffect(() => {
        // Only show connection errors after initial data has been loaded
        if (pivotInfo === null) return;
        
        if (wsError) {
            setError('Connection error');
        } else if (connected === false) {
            setError('Connection lost');
        } else {
            // Clear error when connected
            setError(null);
        }
    }, [connected, wsError, pivotInfo]);

    const getRoleIcon = () => {
        if (!pivotInfo) return null;
        switch (pivotInfo.role) {
            case 'pivot':
                return <IconServer />;
            case 'mixed':
                return <IconServer />;
            case 'node':
                return <IconCloud />;
            default:
                return <IconCloudOff />;
        }
    };

    const getRoleLabel = () => {
        if (!pivotInfo) return 'Loading...';
        switch (pivotInfo.role) {
            case 'pivot':
                return 'Pivot Server';
            case 'mixed':
                return 'Mixed Role Server';
            case 'node':
                return 'Node Server';
            default:
                return 'Not in Cluster';
        }
    };

    const getRoleDescription = () => {
        if (!pivotInfo) return '';
        switch (pivotInfo.role) {
            case 'pivot':
                return 'This server is the central pivot that nodes synchronize with.';
            case 'mixed':
                return `This server is both a pivot for some keys and a node syncing from ${pivotInfo.pivotIP || 'other pivots'}.`;
            case 'node':
                return `This server synchronizes with pivot at ${pivotInfo.pivotIP}`;
            default:
                return 'This server is not configured for pivot synchronization.';
        }
    };

    const getHealthyCount = () => {
        if (!pivotInfo || !pivotInfo.nodes) return 0;
        return pivotInfo.nodes.filter(n => n.healthy).length;
    };

    const getUnhealthyCount = () => {
        if (!pivotInfo || !pivotInfo.nodes) return 0;
        return pivotInfo.nodes.filter(n => !n.healthy).length;
    };

    // Show loading state for both initial load and connection issues
    if (loading || error) {
        return (
            <div className="pivot-status">
                <div className="pivot-status-header">
                    <h3>Pivot Status</h3>
                    <button className="close-btn" onClick={onClose}><IconX /></button>
                </div>
                <div className="pivot-status-content">
                    <div className="pivot-loading">
                        <div className="pivot-loading-text">
                            {error ? 'Reconnecting...' : 'Loading...'}
                        </div>
                        <div className="pivot-loading-bar">
                            <div className="pivot-loading-bar-fill"></div>
                        </div>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="pivot-status">
            <div className="pivot-status-header">
                <h3>Pivot Status</h3>
                <button className="close-btn" onClick={onClose}><IconX /></button>
            </div>
            <div className="pivot-status-content">
                <div className="pivot-role">
                    <div className="role-icon">{getRoleIcon()}</div>
                    <div className="role-info">
                        <div className="role-label">{getRoleLabel()}</div>
                        <div className="role-description">{getRoleDescription()}</div>
                    </div>
                </div>

                {pivotInfo.role !== 'none' && pivotInfo.syncedKeys && pivotInfo.syncedKeys.length > 0 && (
                    <div className="pivot-section">
                        <h4>Synced Keys</h4>
                        <div className="synced-keys">
                            {pivotInfo.syncedKeys.map((key, i) => (
                                <span key={i} className="synced-key">{key}</span>
                            ))}
                        </div>
                    </div>
                )}

                {(pivotInfo.role === 'pivot' || pivotInfo.role === 'mixed') && pivotInfo.nodes && (
                    <div className="pivot-section">
                        <h4>
                            Node Health
                            <span className="node-summary">
                                <span className="healthy-count">{getHealthyCount()} healthy</span>
                                {getUnhealthyCount() > 0 && (
                                    <span className="unhealthy-count">{getUnhealthyCount()} unhealthy</span>
                                )}
                            </span>
                        </h4>
                        {pivotInfo.nodes.length === 0 ? (
                            <div className="no-nodes">No nodes registered</div>
                        ) : (
                            <div className="node-list">
                                {pivotInfo.nodes.map((node, i) => (
                                    <div key={i} className={`node-item ${node.healthy ? 'healthy' : 'unhealthy'}`}>
                                        <div className="node-status-icon">
                                            {node.healthy ? <IconCheck /> : <IconAlertCircle />}
                                        </div>
                                        <div className="node-info">
                                            <div className="node-address">{node.address}</div>
                                            <div className="node-last-check">
                                                Last check: {node.lastCheck || 'Never'}
                                            </div>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                )}

                {(pivotInfo.role === 'node' || pivotInfo.role === 'mixed') && pivotInfo.pivotIP && (
                    <div className="pivot-section">
                        <h4>Pivot Connection</h4>
                        <div className={`node-item ${pivotInfo.pivotHealthy ? 'healthy' : 'unhealthy'}`}>
                            <div className="node-status-icon">
                                {pivotInfo.pivotHealthy ? <IconCheck /> : <IconAlertCircle />}
                            </div>
                            <div className="node-info">
                                <div className="node-address">{pivotInfo.pivotIP}</div>
                                <div className="node-last-check">
                                    Last check: {pivotInfo.pivotLastCheck || 'Never'}
                                </div>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}

window.PivotStatus = PivotStatus;
