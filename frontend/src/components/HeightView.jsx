import React, { useState, useEffect, useRef, useMemo } from 'react';
import { Switch, FormControlLabel } from '@mui/material';
import HeightChart from './HeightChart';
import { CHART_PALETTE } from '../constants';

function HeightView({ tags, referenceTagId }) {
  const MAX_HISTORY = 200;
  const FILTER_WINDOW = 7; // Median filter window size

  const [useFilter, setUseFilter] = useState(true);

  // Filter tags that have pressure data
  const validTags = tags.filter(t => t.pressure !== undefined && t.pressure !== null);
  
  // History State: { tagId: { raw: [], filtered: [], buffer: [], lastTs: 0 } }
  const historyRef = useRef({});
  const [forceUpdate, setForceUpdate] = useState(0);

  // 1. Calculate Relative Heights
  const refTag = validTags.find(t => t.id === referenceTagId);
  const refPressure = refTag ? refTag.pressure : null;

  // Memoize processing
  const processedTags = useMemo(() => {
      return validTags.map(tag => {
          let relHeight = 0;
          if (refPressure !== null) {
              relHeight = (refPressure - tag.pressure) * 0.085;
          }
          return { ...tag, relHeight };
      });
  }, [validTags, refPressure]);

  // 2. Update History (Side Effect)
  useEffect(() => {
      // Strictly wait for Reference Tag Data before plotting history
      if (refPressure === null || refPressure === undefined) {
          return; 
      }

      let changed = false;
      processedTags.forEach(tag => {
          if (!historyRef.current[tag.id]) {
              historyRef.current[tag.id] = { 
                  raw: [], 
                  filtered: [], 
                  buffer: [], // Sliding window for median filter
                  lastTs: 0 
              };
          }
          const record = historyRef.current[tag.id];
          
          // Only push if Timestamp is newer
          if (tag.ts > record.lastTs) {
              record.lastTs = tag.ts;
              
              // --- Filtering Logic (Median Filter) ---
              // 1. Push new value to sliding buffer
              record.buffer.push(tag.relHeight);
              if (record.buffer.length > FILTER_WINDOW) {
                  record.buffer.shift();
              }

              // 2. Compute Median
              let filteredVal = tag.relHeight;
              if (record.buffer.length > 0) {
                  // Sort a copy to find median
                  const sorted = [...record.buffer].sort((a, b) => a - b);
                  const mid = Math.floor(sorted.length / 2);
                  filteredVal = sorted[mid];
              }
              // ---------------------------------------

              record.raw.push(tag.relHeight);
              record.filtered.push(filteredVal);

              // Maintain fixed window
              while (record.raw.length > MAX_HISTORY) record.raw.shift();
              while (record.filtered.length > MAX_HISTORY) record.filtered.shift();
              
              changed = true;
          }
      });
      
      if (changed) setForceUpdate(prev => prev + 1);
  }, [processedTags, refPressure]); 

  // 3. Compute Stats & Sort for Display
  const displayData = processedTags.map(tag => {
      const record = historyRef.current[tag.id];
      // Use filtered data for stats if filter is enabled, else raw
      const hist = record ? (useFilter ? record.filtered : record.raw) : [];
      
      let stdDev = 0;
      // Use last 20 samples for StdDev
      const window = hist.slice(-20);
      if (window.length > 1) {
          const mean = window.reduce((a, b) => a + b, 0) / window.length;
          const variance = window.reduce((a, b) => a + Math.pow(b - mean, 2), 0) / (window.length - 1);
          stdDev = Math.sqrt(variance);
      }
      // Display the CURRENT value based on filter choice? 
      // Usually better to show the filtered value in the list too if filtering is on.
      const currentVal = hist.length > 0 ? hist[hist.length - 1] : tag.relHeight;
      
      return { ...tag, displayHeight: currentVal, stdDev };
  });
  
  // Sort by ID for stable UI order
  displayData.sort((a, b) => a.id - b.id);

  // Assign Colors
  const colorMap = {};
  displayData.forEach((t, i) => {
      colorMap[t.id] = CHART_PALETTE[i % CHART_PALETTE.length];
  });

  // Calculate Aggregated Stats (excluding Reference Tag)
  let maxSpread = 0;
  let maxStability = 0;
  
  const nonRefTags = displayData.filter(t => t.id !== referenceTagId);
  if (nonRefTags.length > 0) {
      let minH = Infinity;
      let maxH = -Infinity;
      
      nonRefTags.forEach(t => {
          if (t.displayHeight < minH) minH = t.displayHeight;
          if (t.displayHeight > maxH) maxH = t.displayHeight;
          if (t.stdDev > maxStability) maxStability = t.stdDev;
      });
      
      if (Number.isFinite(minH) && Number.isFinite(maxH)) {
          maxSpread = maxH - minH;
      }
  }

  // Prepare Data for Chart (Proxy object to map 'data' prop)
  const { chartHistory, rawChartHistory } = useMemo(() => {
      const main = {};
      const raw = {};
      
      Object.keys(historyRef.current).forEach(key => {
          if (useFilter) {
              main[key] = { data: historyRef.current[key].filtered };
              raw[key] = { data: historyRef.current[key].raw };
          } else {
              main[key] = { data: historyRef.current[key].raw };
          }
      });
      
      return { 
          chartHistory: main, 
          rawChartHistory: useFilter ? raw : null 
      };
  }, [forceUpdate, useFilter]);

  return (
    <div className="p-4 w-100 h-100 overflow-auto bg-light">
       <div className="d-flex justify-content-between align-items-center mb-3">
            <div>
                <h4 className="mb-0">Height View (Barometric)</h4>
                <small className="text-muted">
                    Ref: {referenceTagId ? referenceTagId.toString(16).toUpperCase() : 'None'}
                </small>
            </div>
            
            <div className="d-flex align-items-center gap-4">
                <FormControlLabel 
                    control={
                        <Switch 
                            checked={useFilter}
                            onChange={(e) => setUseFilter(e.target.checked)}
                            color="primary"
                        />
                    }
                    label="Filter Outliers"
                />

                {referenceTagId && nonRefTags.length > 0 && (
                    <div className="d-flex gap-3 text-end">
                        <div className="border-start ps-3">
                            <div className="h5 mb-0 text-primary">{maxSpread.toFixed(2)}m</div>
                            <small className="text-muted">Max Spread</small>
                        </div>
                        <div className="border-start ps-3">
                            <div className="h5 mb-0 text-danger">±{maxStability.toFixed(2)}m</div>
                            <small className="text-muted">Max Instability</small>
                        </div>
                    </div>
                )}
            </div>
       </div>

       {!referenceTagId && (
           <div className="alert alert-warning py-2">
               Select a Reference Tag (Right Click in Sidebar)
           </div>
       )}
       
       {/* Bar Visualization */}
       <div className="d-flex align-items-end gap-3 mt-3 pb-3 border-bottom" style={{ height: '280px', overflowX: 'auto' }}>
           {displayData.map(tag => (
               <div key={tag.id} className="d-flex flex-column align-items-center" style={{ width: '70px' }}>
                   <div className="mb-1 fw-bold">{tag.displayHeight.toFixed(2)}m</div>
                   <div className="text-muted" style={{fontSize: '0.7em', marginBottom: '4px'}}>±{tag.stdDev.toFixed(2)}m</div>
                   <div 
                      className="shadow-sm"
                      style={{ 
                          width: '40px', 
                          height: `${Math.min(150, Math.max(4, Math.abs(tag.displayHeight) * 30))}px`, 
                          background: tag.id === referenceTagId ? '#4caf50' : colorMap[tag.id],
                          borderRadius: '4px 4px 0 0',
                          opacity: 0.9,
                          transition: 'height 0.3s ease'
                      }} 
                   />
                   <div className="mt-2 small fw-bold">{tag.id.toString(16).toUpperCase()}</div>
                   <div className="small text-muted" style={{fontSize: '0.7em'}}>{tag.pressure.toFixed(0)}Pa</div>
               </div>
           ))}
       </div>

       {/* Live Chart */}
       {referenceTagId && (
           <HeightChart 
                history={chartHistory} 
                rawHistory={rawChartHistory}
                colors={colorMap} 
                height={250} 
                maxItems={MAX_HISTORY}
                updateTick={forceUpdate} 
            />
       )}
    </div>
  );
}

export default HeightView;
