import { describe, expect, it } from 'vitest';
import { Color, TargetKind } from '../api/enums';
import { buildCatalog, pickerConnections } from './catalog';

const catalog = buildCatalog({
  hosts: [
    { alias: 'web1', source: 'c:1' },
    { alias: 'bastion', source: 'c:4' },
  ],
  savedHosts: [
    {
      id: 'h1',
      name: 'Staging box',
      user: 'deploy',
      host: '10.0.4.12',
      port: 2222,
      connId: 'ssh:deploy@10.0.4.12:2222',
    },
  ],
  profiles: [{ id: 'p1', name: 'Backups' }],
  prefs: { 'ssh:bastion': { hideInPicker: true }, 'ssh:web1': { color: Color.ColorGreen } },
});

describe('buildCatalog', () => {
  it('lists Local, then SSH hosts by name whatever their source, then S3', () => {
    expect(catalog.map((c) => [c.id, c.source])).toEqual([
      ['local', 'local'],
      ['ssh:bastion', 'config'],
      ['ssh:deploy@10.0.4.12:2222', 'saved'],
      ['ssh:web1', 'config'],
      ['s3:p1', 's3'],
    ]);
  });

  it('connects a saved host by address and an alias by name', () => {
    const byId = new Map(catalog.map((c) => [c.id, c]));
    expect(byId.get('ssh:deploy@10.0.4.12:2222')?.target).toEqual({
      kind: TargetKind.TargetSSH,
      user: 'deploy',
      host: '10.0.4.12',
      port: 2222,
    });
    expect(byId.get('ssh:web1')?.target).toEqual({ kind: TargetKind.TargetSSH, alias: 'web1' });
    expect(byId.get('ssh:web1')?.prefs.color).toBe(Color.ColorGreen);
  });

  it('leaves hidden connections out of the picker', () => {
    expect(pickerConnections(catalog).map((c) => c.id)).not.toContain('ssh:bastion');
    expect(pickerConnections(catalog)).toHaveLength(4);
  });
});
