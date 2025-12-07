import React, { useRef } from 'react';

function HeightChart({ history, rawHistory = null, colors, height = 200, maxItems = 200 }) {
    // history: { tagId: { data: [v1...], lastTs: ... } }
    const tagIds = Object.keys(history).sort((a,b) => parseInt(a) - parseInt(b));
    const yBounds = useRef({ min: 0, max: 0, initialized: false });

    if (tagIds.length === 0) return null;

    // Flatten to find min/max (include rawHistory if present to ensure bounds cover outliers)
    let minVal = Infinity;
    let maxVal = -Infinity;
    
    const checkBounds = (histMap) => {
        if (!histMap) return;
        Object.values(histMap).forEach(entry => {
            if (entry && entry.data) {
                entry.data.forEach(v => {
                    if (v < minVal) minVal = v;
                    if (v > maxVal) maxVal = v;
                });
            }
        });
    };

    checkBounds(history);
    if (rawHistory) checkBounds(rawHistory);

    if (!Number.isFinite(minVal)) return <div className="text-center text-muted p-4">Waiting for data...</div>;
    
    // Add padding and enforce minimum range to prevent noise zoom
    let range = maxVal - minVal;
    const MIN_RANGE = 3.0;
    
    if (range < MIN_RANGE) {
        const mid = (minVal + maxVal) / 2;
        const center = Number.isFinite(mid) ? mid : 0;
        minVal = center - MIN_RANGE / 2;
        maxVal = center + MIN_RANGE / 2;
        range = MIN_RANGE;
    }
    
    const padding = range * 0.1; 
    let targetMin = minVal - padding;
    let targetMax = maxVal + padding;

    // Smooth Y-Axis Transitions
    if (!yBounds.current.initialized) {
        yBounds.current.min = targetMin;
        yBounds.current.max = targetMax;
        yBounds.current.initialized = true;
    } else {
        // Lerp factor
        const alpha = 0.1;
        yBounds.current.min += (targetMin - yBounds.current.min) * alpha;
        yBounds.current.max += (targetMax - yBounds.current.max) * alpha;
    }

    const yMin = yBounds.current.min;
    const yMax = yBounds.current.max;
    const yRange = yMax - yMin;
    
    const width = 1000; // Virtual width
    const WINDOW_SIZE = maxItems; 

    // Layout Metrics
    const chartLeft = 60;
    const chartRight = width - 10;
    const chartWidth = chartRight - chartLeft;

    // Generate Y-Ticks (Fixed 0.5m / 50cm intervals)
    const ticks = [];
    const step = 0.5;
    const epsilon = 0.0001;
    const startTick = Math.ceil((yMin - epsilon) / step) * step;
    
    for (let val = startTick; val <= yMax + epsilon; val += step) {
        // Fix floating point artifacts (e.g. 1.0000000002 -> 1.0)
        const cleanVal = Math.round(val * 100) / 100;
        ticks.push(cleanVal);
    }
    
    // Safety: if for some reason range is huge and we have too many ticks, limit them?

    // Generate X-Ticks (5 ticks: 0, -50, -100, -150, -200)
    const xTicks = [];
    for (let i = 0; i < 5; i++) {
        const sampleOffset = Math.round((WINDOW_SIZE - 1) * (i / 4));
        xTicks.push(sampleOffset);
    }
    
    const getPoints = (data) => {
        const len = data.length;
        return data.map((val, idx) => {
            const stepsFromRight = len - 1 - idx;
            const x = chartLeft + chartWidth - (stepsFromRight / Math.max(1, WINDOW_SIZE - 1)) * chartWidth;
            const normY = (val - yMin) / (yRange || 1);
            const y = height - (normY * height);
            return `${x.toFixed(1)},${y.toFixed(1)}`;
        }).join(' ');
    };
    
    return (
        <div className="mt-4 border rounded bg-white p-2 shadow-sm">
            <div className="d-flex justify-content-between align-items-center px-2">
                 <h6 className="text-muted mb-0">Live History ({WINDOW_SIZE} samples)</h6>
            </div>
            <svg viewBox={`0 0 ${width} ${height}`} style={{ width: '100%', height: `${height}px` }} preserveAspectRatio="none">
                {/* X-Axis Grid & Labels */}
                {xTicks.map((offset, i) => {
                    // map offset (0..200) to x position (Right to Left)
                    // offset 0 (Now) -> Right Edge
                    // offset 200 (Old) -> Left Edge
                    const x = chartLeft + chartWidth - (offset / Math.max(1, WINDOW_SIZE - 1)) * chartWidth;
                    
                    return (
                        <g key={`x-${i}`}>
                            <line 
                                x1={x} y1={0} 
                                x2={x} y2={height} 
                                stroke="#f0f0f0" strokeWidth="1" 
                            />
                            <text 
                                x={x} y={height - 6} 
                                textAnchor="middle" 
                                fontSize="10" fill="#999"
                            >
                                {offset === 0 ? "Now" : `-${offset}`}
                            </text>
                        </g>
                    );
                })}

                {/* Y-Axis Ticks & Grid */}
                {ticks.map((tick, i) => {
                    const normY = (tick - yMin) / (yRange || 1);
                    const y = height - (normY * height);
                    // Clamp y to avoid drawing off-canvas if slightly out
                    const safeY = Math.max(10, Math.min(height - 5, y));
                    
                    return (
                        <g key={`y-${i}`}>
                            <line 
                                x1={chartLeft} y1={y} 
                                x2={width} y2={y} 
                                stroke="#eee" strokeWidth="1" 
                            />
                            <text 
                                x={chartLeft - 10} y={safeY + 4} 
                                textAnchor="end" 
                                fontSize="12" fill="#666"
                            >
                                {tick.toFixed(2)}m
                            </text>
                        </g>
                    );
                })}

                {/* Zero Line if visible */}
                {yMin < 0 && yMax > 0 && (
                    <line 
                        x1={chartLeft} 
                        y1={height - ((0 - yMin) / yRange) * height} 
                        x2={width} 
                        y2={height - ((0 - yMin) / yRange) * height} 
                        stroke="#333" 
                        strokeWidth="1" 
                        strokeDasharray="4 4" 
                    />
                )}
                
                {tagIds.map(id => {
                    // Render Raw Data (if available)
                    let rawPolyline = null;
                    if (rawHistory && rawHistory[id] && rawHistory[id].data) {
                        const rawPoints = getPoints(rawHistory[id].data);
                        rawPolyline = (
                            <polyline 
                                key={`raw-${id}`}
                                points={rawPoints} 
                                fill="none" 
                                stroke={colors[id] || '#000'} 
                                strokeWidth="1" 
                                strokeDasharray="4 4"
                                opacity="0.3"
                            />
                        );
                    }

                    const entry = history[id];
                    if (!entry || !entry.data) return rawPolyline;
                    
                    const points = getPoints(entry.data);
                    
                    return (
                        <React.Fragment key={id}>
                            {rawPolyline}
                            <polyline 
                                points={points} 
                                fill="none" 
                                stroke={colors[id] || '#000'} 
                                strokeWidth="2" 
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                opacity="0.8"
                            />
                        </React.Fragment>
                    );
                })}
                
                {/* Axis Line */}
                <line x1={chartLeft} y1={0} x2={chartLeft} y2={height} stroke="#ccc" strokeWidth="1" />
            </svg>
             <div className="d-flex flex-wrap gap-3 justify-content-center mt-2">
                {tagIds.map(id => (
                    <div key={id} className="d-flex align-items-center small">
                        <div style={{width: 12, height: 12, background: colors[id] || '#000', marginRight: 6, borderRadius: 2}}></div>
                        <strong>{parseInt(id).toString(16).toUpperCase()}</strong>
                    </div>
                ))}
            </div>
        </div>
    );
}

export default HeightChart;