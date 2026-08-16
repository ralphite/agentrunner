import {
  Box, BookOpen, FileText, History, Layers, List, Zap, Compass, StickyNote, Map,
} from "lucide-react";
import type { IconName } from "./app-data";
import type { LucideIcon } from "lucide-react";

export const ICONS: Record<IconName, LucideIcon> = {
  page: FileText,
  ws: Zap,
  plan: List,
  research: BookOpen,
  backlog: History,
  layers: Layers,
  cube: Box,
  compass: Compass,
  note: StickyNote,
  map: Map,
};

export function DocIcon({ name, size = 16, className = "ticon" }: { name: IconName; size?: number; className?: string }) {
  const I = ICONS[name];
  return <I size={size} strokeWidth={1.5} className={className} />;
}
