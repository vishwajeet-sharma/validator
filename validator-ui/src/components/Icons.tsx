import {
  LayoutDashboard,
  Lightbulb,
  PlusCircle,
  Moon,
  Sun,
  MessageSquare,
  Youtube,
  Newspaper,
  ExternalLink,
  CheckCircle2,
  XCircle,
  Clock,
  TrendingUp,
  TrendingDown,
  Search,
  ChevronDown,
  Copy,
  Check,
  Sparkles,
  Zap,
  Target,
  Shield,
  AlertTriangle,
  Send,
  X,
  ArrowLeft,
  Plus,
  Globe,
  Trash2,
  Edit3,
} from 'lucide-react';

const ValidatorLogo = ({ className }: { className?: string }) => (
  <svg className={className} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path
      d="M12 2L2 7L12 12L22 7L12 2Z"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
    <path
      d="M2 17L12 22L22 17"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
    <path
      d="M2 12L12 17L22 12"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

const InstagramIcon = ({ className }: { className?: string }) => (
  <svg className={className} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <rect x="2" y="2" width="20" height="20" rx="5" stroke="currentColor" strokeWidth="2" />
    <circle cx="12" cy="12" r="5" stroke="currentColor" strokeWidth="2" />
    <circle cx="17.5" cy="6.5" r="1.5" fill="currentColor" />
  </svg>
);

const TwitterIcon = ({ className }: { className?: string }) => (
  <svg className={className} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path
      d="M4 4L11.5 13.5L4 21H5.5L12.5 14.5L18 21H20L13 13L19.5 4H18L11.5 11.5L6 4H4Z"
      fill="currentColor"
    />
  </svg>
);

export {
  LayoutDashboard,
  Lightbulb,
  PlusCircle,
  Moon,
  Sun,
  MessageSquare,
  Youtube,
  Newspaper,
  ExternalLink,
  CheckCircle2,
  XCircle,
  Clock,
  TrendingUp,
  TrendingDown,
  Search,
  ChevronDown,
  Copy,
  Check,
  Sparkles,
  Zap,
  Target,
  Shield,
  AlertTriangle,
  Send,
  X,
  ArrowLeft,
  Plus,
  Globe,
  Trash2,
  Edit3,
  ValidatorLogo,
  InstagramIcon,
  TwitterIcon,
};
