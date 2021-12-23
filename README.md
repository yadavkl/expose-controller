# expose-controller
expose controller, when deployment created service and ingress will be created

How to test

1. git clone repository
2. cd expose-controller
3. build expose-controller 
   GOOS=linux go build
4. create docker image: docker build -t xxxxxxx/expose-controller:v1.101 
5. Push image to reposory
6. create namespace expose
7. cd manifests
8. kubectl apply -f sa.yaml
9. kubectl apply -f cr.yaml
10. kubectl apply -f crb.yaml


#To test 
1. Create a namespace exp: test
2. created deployment
3. check for service & ingress 
4. delete deployment
5. check for service & ingress.

#Improvements
1. service and ingress name, ports etc
2. handling ingress & service when manually deleted.
