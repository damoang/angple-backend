"""
Angple Backend Load Test

Usage:
    # Local with Web UI
    locust -f locustfile.py --host=http://localhost:8081

    # Headless mode (CI)
    locust -f locustfile.py --host=http://localhost:8081 \
           --users 50 --spawn-rate 10 --run-time 30s --headless

    # Distributed mode
    locust -f locustfile.py --master
    locust -f locustfile.py --worker --master-host=<master-ip>
"""

from locust import HttpUser, task, between, events
import logging

# Suppress verbose logging
logging.getLogger("urllib3").setLevel(logging.WARNING)


class AngpleUser(HttpUser):
    """Simulates typical user behavior on Angple"""

    wait_time = between(1, 3)  # Wait 1-3 seconds between tasks

    def on_start(self):
        """Called when a user starts"""
        # SSR User-Agent to bypass rate limiter during load testing
        self.client.headers["User-Agent"] = "Angple-Web-SSR/LoadTest"
        # Verify API is healthy
        response = self.client.get("/health")
        if response.status_code != 200:
            logging.error("API health check failed")

    @task(10)
    def get_posts_list(self):
        """Browse post list - most common action"""
        self.client.get(
            "/api/v1/boards/free/posts",
            params={"page": 1, "per_page": 20},
            name="/api/v1/boards/[board_id]/posts"
        )

    @task(5)
    def get_post_detail(self):
        """View single post"""
        self.client.get(
            "/api/v1/boards/free/posts/1",
            name="/api/v1/boards/[board_id]/posts/[id]"
        )

    @task(3)
    def get_comments(self):
        """View comments on a post"""
        self.client.get(
            "/api/v1/boards/free/posts/1/comments",
            name="/api/v1/boards/[board_id]/posts/[post_id]/comments"
        )

    @task(2)
    def get_recommended(self):
        """Get recommended posts"""
        self.client.get(
            "/api/v1/recommended/ai/weekly",
            name="/api/v1/recommended/ai/[period]"
        )

    @task(2)
    def get_menus(self):
        """Get sidebar menus"""
        self.client.get("/api/v1/menus/sidebar")


class HealthCheckUser(HttpUser):
    """Lightweight user for basic health monitoring"""

    wait_time = between(5, 10)
    weight = 1  # Lower weight than main user

    def on_start(self):
        self.client.headers["User-Agent"] = "Angple-Web-SSR/LoadTest"

    @task
    def health_check(self):
        """Simple health check"""
        self.client.get("/health")


# Event hooks for custom reporting
@events.request.add_listener
def on_request(request_type, name, response_time, response_length, response, **kwargs):
    """Log slow requests"""
    if response_time > 1000:  # > 1 second
        logging.warning(f"Slow request: {name} took {response_time}ms")


@events.quitting.add_listener
def on_quitting(environment, **kwargs):
    """Print summary on exit"""
    if environment.stats.total.fail_ratio > 0.01:
        logging.error(f"Test failed: {environment.stats.total.fail_ratio:.2%} failure rate")
