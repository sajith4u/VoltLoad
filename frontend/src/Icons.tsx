// Small stroke icons for the sidebar (inline SVG, no icon dependency).
const paths = {
    bolt: <path d="M13 2 4 14h7l-1 8 9-12h-7z"/>,
    grid: <><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></>,
    gauge: <><path d="M3.5 17a9 9 0 1 1 17 0"/><path d="M12 14l4-5"/></>,
    plus: <path d="M12 5v14M5 12h14"/>,
    activity: <polyline points="3 12 7 12 10 4 14 20 17 12 21 12"/>,
    key: <><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></>,
    logout: <path d="M9 4H5v16h4M16 8l4 4-4 4M20 12H9"/>,
};

export default function Icon({name}: { name: keyof typeof paths }) {
    return (
        <svg className="icon" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor"
             strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>
    );
}
