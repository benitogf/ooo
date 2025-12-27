function JsonEditorWrapper({ content, editorRef }) {
  const { useEffect, useRef } = React;
  const containerRef = useRef(null);
  const instanceRef = useRef(null);
  const contentRef = useRef({ json: {} });

  useEffect(() => {
    if (!containerRef.current || typeof JSONEditor === 'undefined') return;
    
    if (instanceRef.current) {
      instanceRef.current.destroy();
      instanceRef.current = null;
    }
    
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
    contentRef.current = { json: jsonData };

    instanceRef.current = new JSONEditor({
      target: containerRef.current,
      props: {
        content: contentRef.current,
        mode: 'tree',
        mainMenuBar: true,
        navigationBar: true,
        statusBar: true,
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
  }, [content]);

  return <div ref={containerRef} className="jse-theme-dark editor-container"></div>;
}

window.JsonEditorWrapper = JsonEditorWrapper;
