function EndpointsList({ clockConnected }) {
  const { useState, useEffect, useCallback, useRef } = React;
  const { IconRefresh, IconPlay, IconChevronDown, IconChevronRight } = window.Icons;
  const JsonEditorWrapper = window.JsonEditorWrapper;
  const editorRef = useRef(null);
  const prevConnected = useRef(clockConnected);

  const [endpoints, setEndpoints] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedEndpoint, setSelectedEndpoint] = useState(null);
  const [selectedMethod, setSelectedMethod] = useState(null);
  const [requestBody, setRequestBody] = useState({});
  const [response, setResponse] = useState(null);
  const [calling, setCalling] = useState(false);
  const [varValues, setVarValues] = useState({});
  const [paramValues, setParamValues] = useState({});
  const [requestSchemaExpanded, setRequestSchemaExpanded] = useState(false);
  const [responseSchemaExpanded, setResponseSchemaExpanded] = useState(false);
  const [responseExpanded, setResponseExpanded] = useState(true);
  const ReactJson = window.reactJsonView ? window.reactJsonView.default : null;

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const data = await Api.fetchEndpoints();
      setEndpoints(data || []);
    } catch (err) {
      console.error('Failed to load endpoints:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Reload when clock reconnects
  useEffect(() => {
    if (clockConnected && !prevConnected.current) {
      loadData();
    }
    prevConnected.current = clockConnected;
  }, [clockConnected, loadData]);

  const selectEndpoint = (endpoint, method) => {
    setSelectedEndpoint(endpoint);
    setSelectedMethod(method);
    setResponse(null);
    setVarValues({});
    setParamValues({});
    setRequestSchemaExpanded(false);
    setResponseSchemaExpanded(false);
    setResponseExpanded(true);
    if (method.request) {
      setRequestBody(method.request);
    } else {
      setRequestBody({});
    }
  };

  const buildPath = (path) => {
    let result = path;
    // Replace route variables in path
    Object.entries(varValues).forEach(([key, value]) => {
      result = result.replace(`{${key}}`, encodeURIComponent(value) || `{${key}}`);
    });
    return result;
  };

  const buildFullUrl = (path) => {
    let url = buildPath(path);
    // Append query parameters
    const queryParams = Object.entries(paramValues)
      .filter(([_, value]) => value && value.trim() !== '')
      .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
      .join('&');
    if (queryParams) {
      url += '?' + queryParams;
    }
    return url;
  };

  // Check if a param value is valid for URL
  const isValidUrlParam = (value) => {
    if (!value || value.trim() === '') return false;
    // Disallow characters that would break URLs or cause issues
    // Allow alphanumeric, dash, underscore, dot, tilde (RFC 3986 unreserved)
    return /^[a-zA-Z0-9\-._~]+$/.test(value);
  };

  // Check if all required route variables are provided and valid
  const areVarsValid = () => {
    if (!selectedEndpoint || !selectedEndpoint.vars) return true;
    const varNames = Object.keys(selectedEndpoint.vars);
    if (varNames.length === 0) return true;
    
    return varNames.every(name => {
      const value = varValues[name];
      return value && isValidUrlParam(value);
    });
  };

  // Get validation error for a specific route variable
  const getVarError = (name) => {
    const value = varValues[name];
    if (!value || value.trim() === '') return null; // Don't show error for empty (just disable button)
    if (!isValidUrlParam(value)) return 'Invalid characters (use only letters, numbers, -, _, ., ~)';
    return null;
  };

  const callEndpoint = async () => {
    if (!selectedEndpoint || !selectedMethod) return;
    if (!areVarsValid()) return;
    setCalling(true);
    setResponse(null);
    try {
      const path = buildFullUrl(selectedEndpoint.path);
      let body = undefined;
      if (selectedMethod.method !== 'GET' && selectedMethod.method !== 'DELETE') {
        // Get content from editor if available
        if (editorRef.current && editorRef.current.getContent) {
          const content = editorRef.current.getContent();
          body = content.json || content.text || requestBody;
        } else {
          body = requestBody;
        }
      }
      const result = await Api.callEndpoint(path, selectedMethod.method, body);
      setResponse(result);
    } catch (err) {
      setResponse({ status: 0, ok: false, data: err.message });
    } finally {
      setCalling(false);
    }
  };

  const getMethodBadgeClass = (method) => {
    switch (method) {
      case 'GET': return 'badge-get';
      case 'POST': return 'badge-post';
      case 'PUT': return 'badge-put';
      case 'PATCH': return 'badge-patch';
      case 'DELETE': return 'badge-delete';
      default: return 'badge-custom';
    }
  };

  const getTypeString = (value) => {
    if (value === null || value === undefined) return 'null';
    if (Array.isArray(value)) {
      if (value.length === 0) return 'array';
      return `array<${getTypeString(value[0])}>`;
    }
    if (typeof value === 'object') return 'object';
    return typeof value;
  };

  const formatSchema = (schema, indent = 0) => {
    if (!schema || typeof schema !== 'object') return getTypeString(schema);
    
    const spaces = '  '.repeat(indent);
    const lines = ['{'];
    
    Object.entries(schema).forEach(([key, value]) => {
      if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
        lines.push(`${spaces}  "${key}": ${formatSchema(value, indent + 1)}`);
      } else {
        lines.push(`${spaces}  "${key}": ${getTypeString(value)}`);
      }
    });
    
    lines.push(`${spaces}}`);
    return lines.join('\n');
  };

  return (
    <div className="container">
      <div className="page-header">
        <div className="page-header-left">
          <h1 className="page-title">Custom Endpoints</h1>
          <div className="stats-row">
            <div className="stat-item">
              <span className="dot blue"></span>
              <span>{endpoints.length}</span> Endpoints
            </div>
          </div>
        </div>
        <div className="header-actions">
          <button className="refresh-btn" onClick={loadData} title="Refresh">
            <IconRefresh />
          </button>
        </div>
      </div>

      <div className="endpoints-layout">
        <div className={`endpoints-list ${loading ? 'loading' : ''}`}>
          {endpoints.length === 0 ? (
            <div className="empty-state">
              <div>No custom endpoints registered</div>
            </div>
          ) : (
            endpoints.map((endpoint, idx) => (
              <div 
                key={idx} 
                className={`endpoint-card ${selectedEndpoint === endpoint ? 'selected' : ''}`}
                onClick={() => selectEndpoint(endpoint, endpoint.methods[0])}
              >
                <div className="endpoint-path">{endpoint.path}</div>
                {endpoint.description && (
                  <div className="endpoint-description">{endpoint.description}</div>
                )}
                <div className="endpoint-methods">
                  {endpoint.methods.map((method, midx) => (
                    <button
                      key={midx}
                      className={`method-btn ${getMethodBadgeClass(method.method)} ${selectedEndpoint === endpoint && selectedMethod === method ? 'active' : ''}`}
                      onClick={(e) => { e.stopPropagation(); selectEndpoint(endpoint, method); }}
                    >
                      {method.method}
                    </button>
                  ))}
                </div>
              </div>
            ))
          )}
        </div>

        {selectedEndpoint && selectedMethod && (
          <div className="endpoint-tester">
            <div className="tester-header">
              <span className={`method-badge ${getMethodBadgeClass(selectedMethod.method)}`}>
                {selectedMethod.method}
              </span>
              <span className="tester-path">{buildFullUrl(selectedEndpoint.path)}</span>
              <button 
                className="btn primary" 
                onClick={callEndpoint} 
                disabled={calling || !areVarsValid()}
              >
                {calling ? 'Calling...' : 'Send'}
                {!calling && <IconPlay />}
              </button>
            </div>

            {selectedEndpoint.vars && Object.keys(selectedEndpoint.vars).length > 0 && (
              <div className="tester-section">
                <div className="section-title">Route Variables <span className="required-hint">(required)</span></div>
                <div className="params-grid">
                  {Object.entries(selectedEndpoint.vars).map(([name, desc]) => {
                    const error = getVarError(name);
                    const isEmpty = !varValues[name] || varValues[name].trim() === '';
                    return (
                      <div key={name} className="param-row">
                        <div className="param-label">
                          <label>{name} <span className="required-star">*</span></label>
                          {desc && desc !== name && <span className="param-desc">{desc}</span>}
                        </div>
                        <input
                          type="text"
                          placeholder={`Enter ${name}`}
                          className={error ? 'input-error' : isEmpty ? 'input-empty' : ''}
                          value={varValues[name] || ''}
                          onChange={(e) => setVarValues(prev => ({ ...prev, [name]: e.target.value }))}
                        />
                        {error && <span className="param-error">{error}</span>}
                        {isEmpty && !error && <span className="param-hint">Required to send request</span>}
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            {selectedMethod.params && Object.keys(selectedMethod.params).length > 0 && (
              <div className="tester-section">
                <div className="section-title">Query Parameters <span className="optional-hint">(optional)</span></div>
                <div className="params-grid">
                  {Object.entries(selectedMethod.params).map(([name, desc]) => {
                    return (
                      <div key={name} className="param-row">
                        <div className="param-label">
                          <label>{name}</label>
                          {desc && desc !== name && <span className="param-desc">{desc}</span>}
                        </div>
                        <input
                          type="text"
                          placeholder={`Enter ${name}`}
                          value={paramValues[name] || ''}
                          onChange={(e) => setParamValues(prev => ({ ...prev, [name]: e.target.value }))}
                        />
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            {selectedMethod.request && (
              <div className="tester-section">
                <div className="section-title">Request Body</div>
                <div className="collapsible-panel">
                  <div 
                    className={`collapsible-header ${!requestSchemaExpanded ? 'collapsed' : ''}`}
                    onClick={() => setRequestSchemaExpanded(!requestSchemaExpanded)}
                  >
                    {requestSchemaExpanded ? <IconChevronDown /> : <IconChevronRight />}
                    <span>View Schema</span>
                  </div>
                  {requestSchemaExpanded && (
                    <div className="collapsible-content">
                      <pre>{formatSchema(selectedMethod.request)}</pre>
                    </div>
                  )}
                </div>
                <div className="editor-container">
                  <JsonEditorWrapper
                    content={requestBody}
                    editorRef={editorRef}
                  />
                </div>
              </div>
            )}

            {selectedMethod.response && (
              <div className="collapsible-panel">
                <div 
                  className={`collapsible-header ${!responseSchemaExpanded ? 'collapsed' : ''}`}
                  onClick={() => setResponseSchemaExpanded(!responseSchemaExpanded)}
                >
                  {responseSchemaExpanded ? <IconChevronDown /> : <IconChevronRight />}
                  <span>Expected Response Schema</span>
                </div>
                {responseSchemaExpanded && (
                  <div className="collapsible-content">
                    <pre>{formatSchema(selectedMethod.response)}</pre>
                  </div>
                )}
              </div>
            )}

            <div className="response-section collapsible-panel">
              <div 
                className={`collapsible-header ${!responseExpanded ? 'collapsed' : ''}`}
                onClick={() => setResponseExpanded(!responseExpanded)}
              >
                {responseExpanded ? <IconChevronDown /> : <IconChevronRight />}
                <span>Response</span>
                {response && (
                  <span className={`status-badge ${response.ok ? 'success' : 'error'}`}>
                    {response.status}
                  </span>
                )}
              </div>
              {responseExpanded && (
                <div className="collapsible-content response-body">
                  {response ? (
                    <div className="json-view-container">
                      {typeof response.data === 'object' && response.data !== null && ReactJson ? (
                        <ReactJson
                          src={response.data}
                          theme="monokai"
                          displayDataTypes={false}
                          displayObjectSize={true}
                          enableClipboard={true}
                          collapsed={false}
                          collapseStringsAfterLength={100}
                          name={false}
                          groupArraysAfterLength={50}
                          style={{ 
                            backgroundColor: 'transparent', 
                            padding: '12px',
                            height: '100%',
                            overflow: 'auto'
                          }}
                        />
                      ) : (
                        <pre className="text-response">{typeof response.data === 'object' ? JSON.stringify(response.data, null, 2) : response.data}</pre>
                      )}
                    </div>
                  ) : (
                    <div className="response-placeholder">
                      <span>Click Send to get a response</span>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

window.EndpointsList = EndpointsList;
