import React from 'react';
import { useLoader } from '@react-three/fiber';
import * as THREE from 'three';

// Explicitly enforcing standard 2D-like coordinate system in 3D World:
// World Origin (0,0,0) corresponds to Map Image Top-Left corner.
// World X+ (Right) corresponds to Map Image Right.
// World Z+ (Forward/Down in top-view) corresponds to Map Image Down.
// This aligns with standard CSS/SVG coordinates where (0,0) is Top-Left.

function MapPlane({ mapConfig }) {
  const texture = useLoader(THREE.TextureLoader, mapConfig.url || '/Map/gongsi.png');
  
  // Texture is loaded with standard settings (flipY=true).
  // PlaneGeometry UVs map (0,0) to bottom-left and (1,1) to top-right.
  // To align Image Top-Left with World (0,0,0) and Image Right/Down with X+/Z+:
  // 1. Create PlaneGeometry of size (W, H). Local coords: [-W/2, W/2], [-H/2, H/2].
  // 2. Rotate -90 degrees around X-axis.
  //    Local +Y (Top) becomes World -Z (North).
  //    Local -Y (Bottom) becomes World +Z (South).
  //    This would put Image Top at -Z. We want Image Top at Z=0.
  // 3. Position the plane such that its Top-Left corner is at (0,0,0).
  //    Center X = W/2. Center Z = H/2.
  //    Position = [W/2, -0.1, H/2].
  
  const centerX = mapConfig.width / 2;
  const centerZ = mapConfig.height / 2;

  return (
    <mesh rotation={[-Math.PI / 2, 0, 0]} position={[centerX, -0.1, centerZ]}>
      <planeGeometry args={[mapConfig.width, mapConfig.height]} />
      <meshBasicMaterial map={texture} toneMapped={false} side={THREE.DoubleSide} />
    </mesh>
  );
}

export default MapPlane;
