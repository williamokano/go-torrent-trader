export interface NewsArticle {
  id: number;
  title: string;
  body: string;
  author_name: string | null;
  created_at: string;
}

export interface AdminNewsArticle extends NewsArticle {
  author_id: number | null;
  published: boolean;
  updated_at: string;
}
