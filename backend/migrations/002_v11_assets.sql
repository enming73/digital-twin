-- v1.1: every station now ships a Blender GLB, not only printer and mounter.
UPDATE assets
SET representation = 'glb',
    model_url = '/models/' || machine_id || '.glb';
