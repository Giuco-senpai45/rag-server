FROM nginx:alpine

# Copy frontend files to nginx server
COPY ./frontend/ /usr/share/nginx/html/

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]