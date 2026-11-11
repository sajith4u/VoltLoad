import {MouseEvent, useState} from 'react';

const W = 360, H = 150, PAD_L = 40, PAD_R = 10, PAD_T = 10, PAD_B = 20;

export interface Point {
    x: number;
    y: number;
    label: string; // shown in the hover readout, e.g. "12.5s" or a run date
}

interface Props {
    title: string;
    unit: string;
    color: string;
    points: Point[];
    area?: boolean; // fill under the line (trend charts)
    dots?: boolean; // mark each point (few points only)
    xEnd?: string;  // label under the right edge of the x axis
}

/** Single-series line chart from a zero baseline, with a hover crosshair and readout. One measure per chart. */
export default function Chart({title, unit, color, points, area, dots, xEnd}: Props) {
    const [hover, setHover] = useState<number | null>(null);
    const xMin = points[0]?.x ?? 0;
    const xSpan = Math.max(1e-9, (points.at(-1)?.x ?? 1) - xMin);
    const yMax = niceMax(Math.max(0, ...points.map(p => p.y)));
    const x = (v: number) => points.length === 1 ? (PAD_L + W - PAD_R) / 2 : PAD_L + (v - xMin) / xSpan * (W - PAD_L - PAD_R);
    const y = (v: number) => H - PAD_B - (v / yMax) * (H - PAD_T - PAD_B);
    const line = points.map((p, i) => `${i ? 'L' : 'M'}${x(p.x).toFixed(1)},${y(p.y).toFixed(1)}`).join('');
    const base = H - PAD_B;

    function onMove(e: MouseEvent<SVGSVGElement>) {
        if (!points.length) return;
        const r = e.currentTarget.getBoundingClientRect();
        const px = (e.clientX - r.left) / r.width * W;
        let best = 0;
        points.forEach((p, i) => { if (Math.abs(x(p.x) - px) < Math.abs(x(points[best].x) - px)) best = i; });
        setHover(best);
    }

    const h = hover !== null ? points[hover] : null;
    const shown = h ?? points.at(-1);
    return (
        <figure className="chart">
            <figcaption>
                <span className="chart-title"><i style={{background: color}}/>{title}</span>
                <span className="chart-readout">
                    {shown ? <><strong>{fmt(shown.y)} {unit}</strong>{h && <em>{h.label}</em>}</> : 'No data'}
                </span>
            </figcaption>
            <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${title} chart`}
                 onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
                {[0, yMax / 2, yMax].map(t => (
                    <g key={t}>
                        <line className="grid" x1={PAD_L} x2={W - PAD_R} y1={y(t)} y2={y(t)}/>
                        <text className="axis" x={PAD_L - 6} y={y(t) + 3} textAnchor="end">{fmt(t)}</text>
                    </g>
                ))}
                {xEnd && <text className="axis" x={W - PAD_R} y={H - 5} textAnchor="end">{xEnd}</text>}
                {area && points.length > 1 &&
                    <path d={`${line}L${x(points.at(-1)!.x).toFixed(1)},${base}L${x(points[0].x).toFixed(1)},${base}Z`} fill={color} opacity={0.18}/>}
                <path className="line" d={line} stroke={color}/>
                {(dots || points.length === 1) && points.map((p, i) =>
                    <circle key={i} className="dot" cx={x(p.x)} cy={y(p.y)} r={3.5} fill={color}/>)}
                {h && <>
                    <line className="crosshair" x1={x(h.x)} x2={x(h.x)} y1={PAD_T} y2={base}/>
                    <circle className="dot" cx={x(h.x)} cy={y(h.y)} r={5} fill={color}/>
                </>}
            </svg>
        </figure>
    );
}

function niceMax(v: number) {
    if (v <= 0) return 1;
    const mag = 10 ** Math.floor(Math.log10(v));
    return [1, 2, 5, 10].map(m => m * mag).find(n => n >= v)!;
}

export const fmt = (v: number) => v >= 100 ? v.toFixed(0) : String(+v.toFixed(v >= 10 ? 1 : 2));
