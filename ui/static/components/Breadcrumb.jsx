function Breadcrumb({ current }) {
  const { IconHome } = window.Icons;
  
  return (
    <div className="breadcrumb">
      <IconHome />
      <span>Home</span>
      <span>›</span>
      <span>{current}</span>
    </div>
  );
}

window.Breadcrumb = Breadcrumb;
