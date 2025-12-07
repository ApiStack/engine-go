import React, { useState, useRef } from 'react';
import { useFrame } from '@react-three/fiber';
import { Html, Line } from '@react-three/drei';

function TagMarker({ id, tagsRef, trailsRef, isTrailEnabled }) {
  const ref = useRef();
  const [layer, setLayer] = useState(0);
  const [trailPoints, setTrailPoints] = useState([]);

  useFrame(() => {
    const tag = tagsRef.current[id];
    if (tag && ref.current) {
      const targetX = tag.x;
      const targetY = tag.z || 0.5;
      const targetZ = tag.y;
      
      ref.current.position.set(targetX, targetY, targetZ);
      
      if (tag.layer !== layer) {
         setLayer(tag.layer);
      }

      // Trail Logic
      if (isTrailEnabled && trailsRef.current[id]) {
          const pts = trailsRef.current[id];
          // Check if update is needed: Length change OR Last point moved
          // This is necessary because the buffer might be full (fixed length), so length check isn't enough.
          let shouldUpdate = false;
          if (pts.length !== trailPoints.length) {
              shouldUpdate = true;
          } else if (pts.length > 0) {
              const lastNew = pts[pts.length - 1];
              const lastOld = trailPoints[trailPoints.length - 1];
              // Simple coordinate comparison
              if (!lastOld || lastNew[0] !== lastOld[0] || lastNew[1] !== lastOld[1] || lastNew[2] !== lastOld[2]) {
                  shouldUpdate = true;
              }
          }

          if (shouldUpdate) {
              setTrailPoints([...pts]);
          }
      } else if (!isTrailEnabled && trailPoints.length > 0) {
          setTrailPoints([]);
      }
    }
  });

  // Removed smoothedPath calculation to strictly follow 'connects adjacent history positions' (Polyline)
  
  return (
    <>
      <group ref={ref}>
        {/* Pin Shape: Cone pointing down + Sphere on top */}
        <group position={[0, 0.25, 0]}>
           {/* Cone: RadiusTop, RadiusBottom, Height, Segments */}
           <mesh position={[0, 0, 0]}>
              <cylinderGeometry args={[0.2, 0, 0.5, 16]} />
              <meshStandardMaterial color="#1976d2" roughness={0.3} metalness={0.5} />
           </mesh>
           {/* Sphere on top */}
           <mesh position={[0, 0.25, 0]}>
              <sphereGeometry args={[0.15, 16, 16]} />
              <meshStandardMaterial color="#1976d2" roughness={0.3} metalness={0.5} />
           </mesh>
        </group>
        
        <Html position={[0, 1.2, 0]} center>
          <div style={{ 
              color: 'white', 
              background: 'rgba(66, 165, 245, 0.85)', 
              padding: '4px 8px', 
              borderRadius: '12px', 
              fontSize: '12px',
              fontWeight: 'bold',
              whiteSpace: 'nowrap',
              cursor: 'pointer',
              border: '1px solid #ccc',
              boxShadow: '0 2px 4px rgba(0,0,0,0.2)'
          }}>
            {id.toString(16).toUpperCase()} (L{layer})
          </div>
        </Html>
      </group>
      {isTrailEnabled && trailPoints.length > 1 && (
         <Line points={trailPoints} color="#ff5722" lineWidth={2} />
      )}
    </>
  );
}

export default TagMarker;
