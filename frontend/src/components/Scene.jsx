import React, { useEffect, useRef } from 'react';
import { OrbitControls } from '@react-three/drei';
import * as THREE from 'three';
import MapPlane from './MapPlane';
import TagMarker from './TagMarker';

function Scene({ tagIds, tagsRef, mapConfig, is2D, focusTagId, setFocusTagId, trailsRef, enabledTrails }) {
  const controlsRef = useRef();

  const newMapCenterX = mapConfig.width / 2;
  const newMapCenterZ = mapConfig.height / 2;

  useEffect(() => {
    if (controlsRef.current) {
      const ctrl = controlsRef.current;
      const camera = ctrl.object;
      
      // Reset standard state
      ctrl.reset(); 

      if (is2D) {
        // 2D Mode: Strictly Top-Down, North-Up
        ctrl.enableRotate = false;
        ctrl.screenSpacePanning = true;
        
        // Fix Camera Orientation:
        // We want to look down the Y axis (at the XZ plane).
        // We want Screen Top to align with -Z (North).
        // Standard LookAt(0,-1,0) with Up(0,1,0) is degenerate.
        // We use Up(0,0,-1).
        camera.up.set(0, 0, -1);
        
        camera.position.set(newMapCenterX, Math.max(mapConfig.width, mapConfig.height) * 1.2, newMapCenterZ);
        camera.lookAt(newMapCenterX, 0, newMapCenterZ);
        
        // Lock to Top-Down View (North-Up)
        ctrl.minPolarAngle = Math.PI / 2;
        ctrl.maxPolarAngle = Math.PI / 2;
      } else {
        // 3D Mode: Standard Perspective
        ctrl.enableRotate = true;
        ctrl.screenSpacePanning = false;
        
        // Restore Standard Up
        camera.up.set(0, 1, 0);
        
        // Angled view (South-East looking North-West)
        camera.position.set(newMapCenterX, 50, newMapCenterZ + 50);
        camera.lookAt(newMapCenterX, 0, newMapCenterZ);
        
        ctrl.minPolarAngle = 0;
        ctrl.maxPolarAngle = Math.PI / 2 - 0.1; // Don't go below ground
      }
      ctrl.target.set(newMapCenterX, 0, newMapCenterZ);
      ctrl.update();
    }
  }, [is2D, newMapCenterX, newMapCenterZ, mapConfig.width, mapConfig.height]);

  // Focus effect
  useEffect(() => {
    if (focusTagId && controlsRef.current && tagsRef.current[focusTagId]) {
        const tag = tagsRef.current[focusTagId];
        const targetX = tag.x;
        const targetZ = tag.y; // Aox Y is World Z

        if (is2D) {
           const currentY = controlsRef.current.object.position.y;
           controlsRef.current.object.position.set(targetX, currentY, targetZ);
        }
        
        controlsRef.current.target.set(targetX, 0, targetZ);
        controlsRef.current.update();
        
        // Reset focus request
        setFocusTagId(null);
    }
  }, [focusTagId, setFocusTagId, tagsRef, is2D]);

  return (
    <>
      <ambientLight intensity={0.8} />
      <pointLight position={[10, 10, 10]} />
      <OrbitControls 
        ref={controlsRef}
        mouseButtons={{
          LEFT: is2D ? THREE.MOUSE.PAN : THREE.MOUSE.ROTATE,
          MIDDLE: THREE.MOUSE.DOLLY,
          RIGHT: is2D ? THREE.MOUSE.ROTATE : THREE.MOUSE.PAN,
        }}
      />
      <gridHelper args={[Math.max(mapConfig.width, mapConfig.height) * 1.5, Math.max(mapConfig.width, mapConfig.height) / 10]} position={[newMapCenterX, -0.2, newMapCenterZ]} />
      <axesHelper args={[5]} position={[0, 0.1, 0]} /> 
      
      {mapConfig.url && (
        <MapPlane mapConfig={mapConfig} />
      )}

      {tagIds.map((id) => (
        <TagMarker 
            key={id} 
            id={id} 
            tagsRef={tagsRef} 
            trailsRef={trailsRef}
            isTrailEnabled={enabledTrails && enabledTrails.has(id)}
        />
      ))}
    </>
  );
}

export default Scene;
