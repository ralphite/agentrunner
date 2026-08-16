import { useState } from "react";
import {
  ChevronRight, MoreHorizontal, Plus, Search, PanelLeft, Target,
  Pencil, Smile, Copy, ArrowRight, Trash2,
} from "lucide-react";
import { TREE, type TreeNode } from "./app-data";
import { DocIcon } from "./icons";
import { useApp } from "./ctx";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import {
  ContextMenu, ContextMenuTrigger, ContextMenuContent, ContextMenuItem, ContextMenuSeparator,
} from "@/components/ui/context-menu";
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip";

function RowMenuItems({ Item, Sep }: { Item: typeof DropdownMenuItem; Sep: typeof DropdownMenuSeparator }) {
  return (
    <>
      <Item><Pencil size={14} strokeWidth={1.5} className="ticon" />Rename</Item>
      <Item><Smile size={14} strokeWidth={1.5} className="ticon" />Change icon</Item>
      <Item><Copy size={14} strokeWidth={1.5} className="ticon" />Duplicate</Item>
      <Item><ArrowRight size={14} strokeWidth={1.5} className="ticon" />Move to…</Item>
      <Sep />
      <Item destructive><Trash2 size={14} strokeWidth={1.5} />Delete</Item>
    </>
  );
}

function Row({ node, depth }: { node: TreeNode; depth: number }) {
  const { doc, openDoc } = useApp();
  const [open, setOpen] = useState(true);
  const hasKids = !!node.kids?.length;

  const icon = <DocIcon name={node.icon} className={node.proj ? "ticon" : "ticon"} />;

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div
            className={`trow ${node.proj ? "proj" : ""} ${doc === node.id ? "active" : ""}`}
            style={{ paddingLeft: 8 + depth * 18 }}
            onClick={() => openDoc(node.id)}
          >
            {hasKids ? (
              <span className="flip">
                <button
                  className={`chev ${open ? "open" : ""}`}
                  onClick={(e) => { e.stopPropagation(); setOpen(!open); }}
                >
                  <ChevronRight size={12} strokeWidth={1.75} />
                </button>
                <span className="ficon">{icon}</span>
              </span>
            ) : (
              <span style={{ margin: "0 1px", display: "grid", placeItems: "center" }}>{icon}</span>
            )}
            <span className="label">{node.label}</span>
            <span className="actions" onClick={(e) => e.stopPropagation()}>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button className="iconbtn"><MoreHorizontal size={14} strokeWidth={1.5} /></button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <RowMenuItems Item={DropdownMenuItem} Sep={DropdownMenuSeparator} />
                </DropdownMenuContent>
              </DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button className="iconbtn"><Plus size={14} strokeWidth={1.5} /></button>
                </TooltipTrigger>
                <TooltipContent>Add a page inside</TooltipContent>
              </Tooltip>
            </span>
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <RowMenuItems Item={ContextMenuItem} Sep={ContextMenuSeparator} />
        </ContextMenuContent>
      </ContextMenu>
      {hasKids && open && node.kids!.map((k) => <Row key={k.id} node={k} depth={depth + 1} />)}
    </>
  );
}

export function Sidebar({ onClose }: { onClose: () => void }) {
  return (
    <nav className="sidebar">
      <div className="side-top">
        <span className="brand">
          <span className="brandmark"><Target size={13} strokeWidth={2} /></span>
          Context Center
        </span>
        <span style={{ flex: 1 }} />
        <Tooltip>
          <TooltipTrigger asChild>
            <button className="iconbtn"><Search size={16} strokeWidth={1.5} /></button>
          </TooltipTrigger>
          <TooltipContent>Search</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <button className="iconbtn" onClick={onClose}><PanelLeft size={16} strokeWidth={1.5} /></button>
          </TooltipTrigger>
          <TooltipContent>Close sidebar</TooltipContent>
        </Tooltip>
      </div>
      <div className="tree">
        {TREE.map((n, i) => (
          <div key={n.id}>
            {i > 0 && <div style={{ height: 8 }} />}
            <Row node={n} depth={0} />
          </div>
        ))}
      </div>
      <div className="newproj">
        <div className="trow">
          <Plus size={16} strokeWidth={1.5} className="ticon" />
          <span className="label">New project</span>
        </div>
      </div>
    </nav>
  );
}
