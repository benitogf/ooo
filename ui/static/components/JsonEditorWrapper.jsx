function JsonEditorWrapper({ content, editorRef, readOnly }) {
  const { useEffect, useRef, useMemo } = React;
  const containerRef = useRef(null);
  const instanceRef = useRef(null);
  const contentRef = useRef({ json: {} });
  const lastContentStrRef = useRef(null);

  const parseContent = (content) => {
    let jsonData = {};
    if (content !== null && content !== undefined) {
      if (typeof content === 'object') {
        jsonData = JSON.parse(JSON.stringify(content));
      } else if (typeof content === 'string') {
        try { jsonData = JSON.parse(content); } 
        catch (e) { jsonData = { value: content }; }
      } else {
        jsonData = { value: content };
      }
    }
    return jsonData;
  };

  // Serialize content for comparison
  const contentStr = useMemo(() => {
    return JSON.stringify(parseContent(content));
  }, [content]);

  // Initialize editor on mount
  useEffect(() => {
    if (!containerRef.current || typeof JSONEditor === 'undefined') return;
    
    const jsonData = parseContent(content);
    contentRef.current = { json: jsonData };
    lastContentStrRef.current = JSON.stringify(jsonData);

    instanceRef.current = new JSONEditor({
      target: containerRef.current,
      props: {
        content: contentRef.current,
        mode: 'tree',
        mainMenuBar: !readOnly,
        navigationBar: true,
        statusBar: true,
        readOnly: readOnly || false,
        onChange: (c) => { contentRef.current = c; }
      }
    });

    if (editorRef) {
      editorRef.current = { getContent: () => contentRef.current };
    }

    return () => {
      if (instanceRef.current) {
        instanceRef.current.destroy();
        instanceRef.current = null;
      }
    };
  }, []);

  // Update content when it changes (without remounting)
  useEffect(() => {
    if (!instanceRef.current) return;
    
    // Only update if content actually changed
    if (contentStr !== lastContentStrRef.current) {
      const jsonData = parseContent(content);
      console.log('JsonEditorWrapper updating content:', jsonData);
      contentRef.current = { json: jsonData };
      instanceRef.current.set(contentRef.current);
      lastContentStrRef.current = contentStr;
    }
  }, [contentStr]);

  // Update readOnly when it changes
  useEffect(() => {
    if (!instanceRef.current) return;
    instanceRef.current.updateProps({ readOnly: readOnly || false, mainMenuBar: !readOnly });
  }, [readOnly]);

  return <div ref={containerRef} className="jse-theme-dark editor-container"></div>;
}

window.JsonEditorWrapper = JsonEditorWrapper;
