import React from 'react';
import BlogPostItemHeaderTitle from '@theme/BlogPostItem/Header/Title';
import BlogPostItemHeaderInfo from '@theme/BlogPostItem/Header/Info';
import BlogPostItemHeaderAuthors from '@theme/BlogPostItem/Header/Authors';
export default function BlogPostItemHeader() {
  return (
    <header>
      <BlogPostItemHeaderTitle />
      <BlogPostItemHeaderAuthors />
      <div style={{height: '1px', background: '#F2F2F2', margin: '15px 0'}}></div>
      <BlogPostItemHeaderInfo />
      <div style={{height: '1px', background: '#F2F2F2', margin: '15px 0'}}></div>
    </header>
  );
}
